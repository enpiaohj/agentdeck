package sessions

import (
	"errors"
	"reflect"
)

func (p *NativeDeletePlan) indices() []*nativeIndex {
	out := append([]*nativeIndex(nil), p.histories...)
	if p.index != nil {
		out = append(out, p.index)
	}
	return out
}

// RebaseAfterOwnBatchChange accepts only changes produced by the successfully
// deleted preceding plans from this batch. Original backup/recovery snapshots
// remain immutable. Call after all backups, immediately before each Delete.
func (p *NativeDeletePlan) RebaseAfterOwnBatchChange(previousPlans []*NativeDeletePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready || p.deleted {
		return errors.New("原始批量删除计划状态无效")
	}
	for _, idx := range p.indices() {
		expected := idx.Hash
		for _, previous := range previousPlans {
			if previous == nil || previous == p {
				return errors.New("原始批量删除计划顺序无效")
			}
			previous.mu.Lock()
			if !previous.deleted {
				previous.mu.Unlock()
				return errors.New("前序原始删除尚未完成")
			}
			for _, old := range previous.indices() {
				if nativeEqualPath(idx.Path, old.Path) && old.removed {
					if old.expected() != expected || old.afterHash == "" {
						previous.mu.Unlock()
						return errors.New("原始共享索引批次校验链不匹配")
					}
					expected = old.afterHash
				}
			}
			previous.mu.Unlock()
		}
		idx.expectedHash = expected
	}
	for _, d := range p.databases {
		expected := make([]nativeTable, len(d.Tables))
		for i, t := range d.Tables {
			expected[i] = t
			expected[i].Inserts = append([]string(nil), t.Inserts...)
		}
		for _, previous := range previousPlans {
			previous.mu.Lock()
			if !previous.deleted {
				previous.mu.Unlock()
				return errors.New("前序原始删除尚未完成")
			}
			removed := map[string]bool{}
			for _, old := range previous.databases {
				if nativeEqualPath(old.Path, d.Path) && old.removed {
					if old.Schema != d.Schema {
						previous.mu.Unlock()
						return errors.New("原始状态库批次结构不匹配")
					}
					for _, t := range old.Tables {
						for _, insert := range t.Inserts {
							removed[insert] = true
						}
					}
				}
			}
			previous.mu.Unlock()
			for i := range expected {
				kept := expected[i].Inserts[:0]
				for _, insert := range expected[i].Inserts {
					if !removed[insert] {
						kept = append(kept, insert)
					}
				}
				expected[i].Inserts = kept
				if len(kept) == 0 {
					expected[i].Inserts = nil
				}
			}
		}
		if !reflect.DeepEqual(d.Tables, expected) {
			d.expectedTables = expected
		}
	}
	return p.validate()
}
