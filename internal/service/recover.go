package service

import (
	"context"
	"fmt"

	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/watermark"
)

// Recover 服务启动时执行的重启恢复：
//  1. 扫描全部非封存手稿；
//  2. 为每份手稿中「有效水印观测 + 模具对相同 + 半片互补」但尚无配对记录的观测对补齐配对候选；
//  3. ctx 取消时停止后续写入，已完成的补齐保留。
//
// 幂等：已存在的配对不重复创建；冻结版本与已确认关系不被改写。
func (s *Service) Recover(ctx context.Context) (*RecoverReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rep := &RecoverReport{}
	ms, err := s.store.ListManuscripts()
	if err != nil {
		return nil, fmt.Errorf("列出手稿以恢复: %w", err)
	}
	for _, m := range ms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if m.Status.IsTerminal() {
			continue
		}
		created, err := s.recoverPairings(ctx, m)
		if err != nil {
			return nil, err
		}
		rep.PairingsCreated += created
		rep.ManuscriptsChecked++
	}
	return rep, nil
}

// recoverPairings 对单份手稿补齐缺失的水印配对候选。
func (s *Service) recoverPairings(ctx context.Context, m *model.Manuscript) (int, error) {
	leaves, err := s.store.ListLeaves(m.ID)
	if err != nil {
		return 0, fmt.Errorf("列出纸页以恢复配对: %w", err)
	}
	type wmEntry struct {
		w    *model.WatermarkObservation
		leaf *model.Leaf
	}
	var obs []wmEntry
	for _, l := range leaves {
		if !l.Status.IsValidObservation() {
			continue
		}
		ws, err := s.store.ListWatermarksByLeaf(l.ID)
		if err != nil {
			return 0, fmt.Errorf("列出纸页水印以恢复配对: %w", err)
		}
		for _, w := range ws {
			if w.Status.ValidObservation() {
				obs = append(obs, wmEntry{w: w, leaf: l})
			}
		}
	}
	created := 0
	for i := 0; i < len(obs); i++ {
		for j := i + 1; j < len(obs); j++ {
			if err := ctx.Err(); err != nil {
				return created, err
			}
			a, b := obs[i], obs[j]
			if a.w.MoldPairID != b.w.MoldPairID {
				continue
			}
			if a.w.Position == b.w.Position {
				continue
			}
			if a.leaf.ID == b.leaf.ID {
				continue
			}
			_, err := s.store.FindPairingByWatermarks(a.w.ID, b.w.ID)
			if err == nil {
				continue
			}
			if !model.IsNotFound(err) {
				return created, fmt.Errorf("查找既有配对: %w", err)
			}
			p, err := watermark.Pair(a.w, b.w, a.leaf.WidthMM)
			if err != nil {
				return created, err
			}
			p.ID = genID()
			p.ManuscriptID = m.ID
			p.Version = 1
			if err := s.store.SavePairing(p); err != nil {
				return created, fmt.Errorf("恢复写入配对: %w", err)
			}
			created++
		}
	}
	return created, nil
}

// RecoverReport 恢复报告。
type RecoverReport struct {
	ManuscriptsChecked int `json:"manuscripts_checked"`
	PairingsCreated    int `json:"pairings_created"`
}

// EnsureManuscript 读取手稿；不存在返回领域错误。
func (s *Service) EnsureManuscript(id string) (*model.Manuscript, error) {
	m, err := s.store.GetManuscript(id)
	if err != nil {
		return nil, err
	}
	return m, nil
}
