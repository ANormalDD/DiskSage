package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"disksage/internal/analyzer"
	"disksage/internal/cleaner"
	"disksage/internal/config"
	"disksage/internal/models"
	"disksage/internal/privilege"
	"disksage/internal/scanner"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const elevationRequiredPrefix = "ELEVATION_REQUIRED"

// App is the thin Wails binding layer.
type App struct {
	scanner   *scanner.Scanner
	analyzer  *analyzer.Analyzer
	cleaner   *cleaner.Cleaner
	configMgr *config.Manager
	ctx       context.Context

	mu       sync.RWMutex
	lastScan *models.DirNode
}

func NewApp() *App {
	cfgMgr := config.NewManager("")
	cfg, _ := cfgMgr.Load()

	s := scanner.NewScanner(models.DefaultScanConfig())
	c := cleaner.NewCleaner(cleaner.Options{})

	app := &App{
		scanner:   s,
		cleaner:   c,
		configMgr: cfgMgr,
	}

	a := analyzer.NewAnalyzer(analyzer.Options{
		Client: makeLLMClient(cfg.LLM),
		Config: cfg.LLM,
		ScanDeeper: func(path string, depth int) (models.DirNode, error) {
			return s.ScanDir(path, depth)
		},
		CheckDirContent: func(path string) (models.FileTypeDistribution, error) {
			return s.CheckDirContent(path)
		},
		OnProgress: func(event analyzer.AnalysisProgressEvent) {
			app.mu.RLock()
			runtimeCtx := app.ctx
			app.mu.RUnlock()
			if runtimeCtx != nil {
				runtime.EventsEmit(runtimeCtx, "analyze:progress", event)
			}
		},
	})
	app.analyzer = a
	return app
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	_ = ctx
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = nil
}

func (a *App) activeContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) ScanDrive(drive string) (models.ScanResult, error) {
	if drive == "" {
		return models.ScanResult{}, errors.New("drive cannot be empty")
	}
	if requiresElevationForScan(drive) && !privilege.IsElevated() {
		return models.ScanResult{}, fmt.Errorf("%s: 扫描 %s 需要管理员权限。请点击“以管理员重启”后重试", elevationRequiredPrefix, drive)
	}

	a.mu.RLock()
	runtimeCtx := a.ctx
	a.mu.RUnlock()
	if runtimeCtx != nil {
		a.scanner.SetProgressCallback(func(progress models.ScanProgress) {
			runtime.EventsEmit(runtimeCtx, "scan:progress", progress)
		})
		defer a.scanner.SetProgressCallback(nil)
	}

	root, err := a.scanner.ScanDrive(drive)
	if err != nil {
		return models.ScanResult{}, err
	}

	a.mu.Lock()
	a.lastScan = &root
	a.mu.Unlock()

	compressed := scanner.RenderCompressedTree(root, scanner.RenderConfig{
		TopNPerLevel: 20,
		MinChildSize: 50 * 1024 * 1024,
	})

	return models.ScanResult{
		Root:       root,
		Compressed: compressed,
	}, nil
}

func (a *App) AnalyzeLastScan() ([]models.Recommendation, error) {
	a.mu.RLock()
	last := a.lastScan
	a.mu.RUnlock()
	if last == nil {
		return nil, errors.New("scan first")
	}
	return a.analyzer.Analyze(a.activeContext(), *last)
}

func (a *App) ContinueAnalyzeLastScan() ([]models.Recommendation, error) {
	return a.analyzer.Continue(a.activeContext())
}

func (a *App) CanContinueAnalyze() bool {
	return a.analyzer.HasPendingAnalysis()
}

func (a *App) Clean(req models.CleanRequest) (models.CleanSummary, error) {
	if len(req.Items) == 0 {
		return models.CleanSummary{}, errors.New("no items selected")
	}
	return a.cleaner.Clean(a.activeContext(), req)
}

func (a *App) GetConfig() (models.AppConfig, error) {
	return a.configMgr.Load()
}

func (a *App) SaveConfig(cfg models.AppConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := a.configMgr.Save(cfg); err != nil {
		return err
	}
	a.analyzer.UpdateConfig(cfg.LLM)
	a.analyzer.SetClient(makeLLMClient(cfg.LLM))
	return nil
}

func (a *App) GetTokenStats() models.TokenStats {
	return a.analyzer.GetTokenStats()
}

func (a *App) GetLLMDebugInfo() models.LLMDebugInfo {
	return a.analyzer.GetDebugInfo()
}

func (a *App) IsElevated() bool {
	return privilege.IsElevated()
}

func (a *App) RequestElevation() error {
	if privilege.IsElevated() {
		return nil
	}
	if err := privilege.RequestElevation(); err != nil {
		return err
	}

	a.mu.RLock()
	runtimeCtx := a.ctx
	a.mu.RUnlock()

	if runtimeCtx != nil {
		go func(ctx context.Context) {
			time.Sleep(300 * time.Millisecond)
			runtime.Quit(ctx)
		}(runtimeCtx)
	}

	return nil
}

func makeLLMClient(cfg models.LLMConfig) analyzer.LLMClient {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if strings.TrimSpace(cfg.APIKey) != "" && (provider == "openai" || provider == "custom" || provider == "openai-compatible") {
		return analyzer.NewOpenAICompatibleClient()
	}
	return analyzer.NewHeuristicClient()
}

func requiresElevationForScan(path string) bool {
	if goruntime.GOOS != "windows" {
		return false
	}
	if path == "" {
		return false
	}
	normalized := strings.TrimSpace(path)
	if strings.EqualFold(normalized, "C:") || strings.EqualFold(normalized, `C:\`) || strings.EqualFold(normalized, `C:/`) {
		return true
	}
	clean := strings.TrimRight(filepath.Clean(normalized), `\\/`)
	if strings.EqualFold(clean, "C:") {
		return true
	}
	return privilege.NeedsElevation(clean)
}
