package app

import "fmt"

func (a *App) checkDeletedMark(key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.deleted[key] {
		return fmt.Errorf("会话已删除，未写入标注")
	}
	return nil
}

// MarkService 是用户标注(收藏/归档/标签/备注)的门面:key 校验在
// marks 包,这里负责写后刷新装配缓存并推 sessions:changed 事件。
type MarkService struct{ a *App }

func (s *MarkService) SetFavorite(key string, v bool) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.checkDeletedMark(key); err != nil {
		return err
	}
	if err := s.a.marks.SetFavorite(s.a.markKey(key), v); err != nil {
		return err
	}
	s.a.changed(key)
	return nil
}

func (s *MarkService) SetArchived(key string, v bool) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.checkDeletedMark(key); err != nil {
		return err
	}
	if err := s.a.marks.SetArchived(s.a.markKey(key), v); err != nil {
		return err
	}
	s.a.changed(key)
	return nil
}

func (s *MarkService) SetTags(key string, tags []string) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.checkDeletedMark(key); err != nil {
		return err
	}
	if err := s.a.marks.SetTags(s.a.markKey(key), tags); err != nil {
		return err
	}
	s.a.changed(key)
	return nil
}

func (s *MarkService) SetNote(key, note string) error {
	s.a.dataMu.Lock()
	defer s.a.dataMu.Unlock()
	if err := s.a.checkDeletedMark(key); err != nil {
		return err
	}
	if err := s.a.marks.SetNote(s.a.markKey(key), note); err != nil {
		return err
	}
	s.a.changed(key)
	return nil
}

func (s *MarkService) TagsInUse() ([]string, error) {
	return s.a.marks.TagsInUse()
}

// Get 单个会话的标注。
func (s *MarkService) Get(key string) (Mark, error) {
	m, err := s.a.marks.Get(s.a.markKey(key))
	if err != nil {
		return Mark{}, err
	}
	return Mark{
		Key: key, Favorite: m.Favorite, Archived: m.Archived,
		Tags: m.Tags, Note: m.Note, Updated: m.Updated.UnixMilli(),
	}, nil
}

// Mark 是标注的对外形态(时间用毫秒时间戳,规避 time.Time 绑定差异)。
type Mark struct {
	Key      string   `json:"key"`
	Favorite bool     `json:"favorite"`
	Archived bool     `json:"archived"`
	Tags     []string `json:"tags"`
	Note     string   `json:"note"`
	Updated  int64    `json:"updated"`
}

// changed 刷新缓存快照并广播。
func (a *App) changed(key string) {
	a.refreshMark(a.markKey(key))
	a.emit(EventSessionsChanged, key)
}
