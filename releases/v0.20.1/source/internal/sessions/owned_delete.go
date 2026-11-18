package sessions

// AgentDeck: deleting what AgentDeck itself stored for a session — the copy of an
// imported chat. It is the same plan as a native deletion (a verified, hashed backup
// first, revalidation, restore on failure, never a recursive delete), but the files
// come from a resolver the caller supplies instead of from an agent's folders.

import (
	"errors"
	"sort"
)

// currentPaths is the set of files the plan covers right now.
func (p *NativeDeletePlan) currentPaths() ([]string, error) {
	if p.ownedPaths != nil {
		return p.ownedPaths()
	}
	return nativePaths(p.session)
}

// PrepareOwnedDelete takes the content fingerprints of the files paths names. paths
// must return only absolute paths of regular files the caller owns, and is called
// again to check that the set has not changed; the caller, not this package, is
// what decides ownership. It writes nothing.
func PrepareOwnedDelete(s Session, paths func() ([]string, error)) (*NativeDeletePlan, error) {
	if paths == nil || !nativeID.MatchString(s.ID) {
		return nil, errors.New("会话 ID 无效")
	}
	sorted := func() ([]string, error) {
		list, err := paths()
		if err != nil {
			return nil, err
		}
		out := append([]string(nil), list...)
		sort.Strings(out)
		return out, nil
	}
	list, err := sorted()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errors.New("没有找到这个会话保存在本机的副本")
	}
	p := &NativeDeletePlan{session: s, ownedPaths: sorted}
	for _, path := range list {
		hash, mode, err := nativeHash(path)
		if err != nil {
			return nil, err
		}
		p.files = append(p.files, nativeFile{Path: path, Hash: hash, Mode: mode})
		p.Files = append(p.Files, path)
		p.Remove = append(p.Remove, path)
	}
	return p, nil
}
