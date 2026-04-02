package cleaner

import (
	"context"
	"time"

	"disksage/internal/models"
)

type Options struct {
	HistoryPath string
}

type Cleaner struct {
	deleteStrategy   Strategy
	commandStrategy  Strategy
	redirectStrategy Strategy
	history          *HistoryStore
}

func NewCleaner(opts Options) *Cleaner {
	return &Cleaner{
		deleteStrategy:   DeleteStrategy{},
		commandStrategy:  CommandStrategy{},
		redirectStrategy: RedirectStrategy{},
		history:          NewHistoryStore(opts.HistoryPath),
	}
}

func (c *Cleaner) Clean(ctx context.Context, req models.CleanRequest) (models.CleanSummary, error) {
	summary := models.CleanSummary{StartedAt: time.Now()}
	opts := ExecuteOptions{
		PermanentDelete: req.PermanentDelete,
		ConfirmCommands: req.ConfirmCommands,
	}

	for _, item := range req.Items {
		strategy := c.pickStrategy(item.CleanMethod)
		freed, err := strategy.Execute(ctx, item, opts)
		res := models.ItemCleanResult{
			Path:    item.Path,
			Success: err == nil,
			Freed:   freed,
		}
		if err != nil {
			res.Error = err.Error()
		} else {
			summary.Freed += freed
		}
		summary.Results = append(summary.Results, res)
	}
	summary.EndedAt = time.Now()

	_ = c.history.Append(HistoryRecord{
		Timestamp: time.Now(),
		Requester: req.RequestedBy,
		Summary:   summary,
	})

	return summary, nil
}

func (c *Cleaner) pickStrategy(method models.CleanMethod) Strategy {
	switch method {
	case models.MethodDelete, models.MethodRecycle:
		return c.deleteStrategy
	case models.MethodCommand:
		return c.commandStrategy
	case models.MethodRedirect:
		return c.redirectStrategy
	default:
		return c.redirectStrategy
	}
}
