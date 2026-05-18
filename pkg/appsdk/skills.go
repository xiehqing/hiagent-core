package appsdk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/xiehqing/hiagent-core/internal/config"
	iskills "github.com/xiehqing/hiagent-core/internal/skills"
)

type SkillDetail struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Instructions  string            `json:"instructions"`
	Path          string            `json:"path"`
	SkillFilePath string            `json:"skill_file_path"`
	Builtin       bool              `json:"builtin"`
	Disabled      bool              `json:"disabled"`
	Active        bool              `json:"active"`
}

type ImportSkillResult struct {
	Skill      SkillDetail `json:"skill"`
	TargetPath string      `json:"target_path"`
}

// ImportSkill imports a custom skill into the first default global skills directory.
func ImportSkill(sourcePath string) (*ImportSkillResult, error) {
	globalDirs := config.GlobalSkillsDirs()
	if len(globalDirs) == 0 {
		return nil, fmt.Errorf("sdk.ImportSkill: no global skills directories configured")
	}
	return ImportSkillTo(sourcePath, globalDirs[0])
}

// ImportSkillTo imports a custom skill into the target skills directory.
func ImportSkillTo(sourcePath, targetRoot string) (*ImportSkillResult, error) {
	if sourcePath == "" {
		return nil, fmt.Errorf("sdk.ImportSkill: sourcePath is required")
	}
	if targetRoot == "" {
		return nil, fmt.Errorf("sdk.ImportSkill: targetRoot is required")
	}

	sourceSkillFile, sourceDir, err := resolveSkillSource(sourcePath)
	if err != nil {
		return nil, err
	}

	skill, err := iskills.Parse(sourceSkillFile)
	if err != nil {
		return nil, fmt.Errorf("sdk.ImportSkill: failed to parse skill: %w", err)
	}
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("sdk.ImportSkill: invalid skill format: %w", err)
	}

	if err := ensureSkillNotDuplicate(skill.Name); err != nil {
		return nil, err
	}

	targetDir := filepath.Join(targetRoot, skill.Name)
	if _, err := os.Stat(targetDir); err == nil {
		return nil, fmt.Errorf("sdk.ImportSkill: target skill already exists: %s", skill.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("sdk.ImportSkill: failed to check target skill path: %w", err)
	}

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return nil, fmt.Errorf("sdk.ImportSkill: failed to create target root: %w", err)
	}
	if err := copyDir(sourceDir, targetDir); err != nil {
		return nil, fmt.Errorf("sdk.ImportSkill: failed to copy skill files: %w", err)
	}

	imported, err := iskills.Parse(filepath.Join(targetDir, iskills.SkillFileName))
	if err != nil {
		return nil, fmt.Errorf("sdk.ImportSkill: failed to verify imported skill: %w", err)
	}
	if err := imported.Validate(); err != nil {
		return nil, fmt.Errorf("sdk.ImportSkill: imported skill validation failed: %w", err)
	}

	return &ImportSkillResult{
		Skill:      skillDetailFromInternal(imported, false, false),
		TargetPath: targetDir,
	}, nil
}

// ListDefaultActiveSkillsDetails returns only active skills from the default
// global skills directories.
func ListDefaultActiveSkillsDetails() []SkillDetail {
	return filterActiveSkills(ListDefaultSkillsDetails())
}

// ListDefaultSkillsDetails returns the deduplicated skill list details using
// the default global skills directories.
func ListDefaultSkillsDetails() []SkillDetail {
	return ListSkillsDetailsFromPaths(config.GlobalSkillsDirs(), nil)
}

// ListActiveSkillsDetailsFromPaths returns only active skills for the provided
// skill paths and disabled skill names.
func ListActiveSkillsDetailsFromPaths(skillsPaths []string, disabledSkills []string) []SkillDetail {
	return filterActiveSkills(ListSkillsDetailsFromPaths(skillsPaths, disabledSkills))
}

// ListSkillsDetailsFromPaths returns the deduplicated skill list details for the
// provided skill paths and disabled skill names.
func ListSkillsDetailsFromPaths(skillsPaths []string, disabledSkills []string) []SkillDetail {
	return collectSkillDetails(skillsPaths, disabledSkills)
}

// ListSkillsDetails returns the deduplicated skill list details for the current workspace.
func ListSkillsDetails(ctx context.Context, conn *sql.DB, opts ...Option) ([]SkillDetail, error) {
	if conn == nil {
		return nil, fmt.Errorf("sdk.ListSkillsDetails: conn is required")
	}

	o := &Options{
		cfg: AppConfig{
			Debug: false,
		},
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.cfg.WorkDir == "" {
		return nil, fmt.Errorf("sdk.ListSkillsDetails: WorkDir is required (use sdk.WithWorkDir)")
	}

	o.cfg.DataDir = config.DefaultDataDir(o.cfg.WorkDir, o.cfg.DataDir)
	cfg, err := config.Init(o.cfg.WorkDir, o.cfg.DataDir, conn, o.cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("sdk.ListSkillsDetails: failed to initialize config: %w", err)
	}

	return ListSkillsDetailsFromPaths(cfg.Config().Options.SkillsPaths, cfg.Config().Options.DisabledSkills), nil
}

func collectSkillDetails(skillsPaths []string, disabledSkills []string) []SkillDetail {
	discovered := append([]*iskills.Skill{}, iskills.DiscoverBuiltin()...)
	if len(skillsPaths) > 0 {
		discovered = append(discovered, iskills.Discover(skillsPaths)...)
	}

	all := iskills.Deduplicate(discovered)
	disabledSet := make(map[string]bool, len(disabledSkills))
	for _, name := range disabledSkills {
		disabledSet[name] = true
	}

	result := make([]SkillDetail, 0, len(all))
	for _, skill := range all {
		result = append(result, skillDetailFromInternal(skill, disabledSet[skill.Name], skill.Builtin))
	}

	slices.SortFunc(result, func(a, b SkillDetail) int {
		switch {
		case a.SkillFilePath < b.SkillFilePath:
			return -1
		case a.SkillFilePath > b.SkillFilePath:
			return 1
		default:
			return 0
		}
	})

	return result
}

func copySkillMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func skillDetailFromInternal(skill *iskills.Skill, disabled bool, builtin bool) SkillDetail {
	detail := SkillDetail{
		Name:          skill.Name,
		Description:   skill.Description,
		License:       skill.License,
		Compatibility: skill.Compatibility,
		Metadata:      copySkillMetadata(skill.Metadata),
		Instructions:  skill.Instructions,
		Path:          skill.Path,
		SkillFilePath: skill.SkillFilePath,
		Builtin:       builtin,
		Disabled:      disabled,
	}
	detail.Active = !detail.Disabled
	return detail
}

func filterActiveSkills(details []SkillDetail) []SkillDetail {
	if len(details) == 0 {
		return nil
	}
	result := make([]SkillDetail, 0, len(details))
	for _, detail := range details {
		if detail.Active {
			result = append(result, detail)
		}
	}
	return result
}

func resolveSkillSource(sourcePath string) (skillFile string, skillDir string, err error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("sdk.ImportSkill: failed to stat source path: %w", err)
	}

	if info.IsDir() {
		skillFile = filepath.Join(sourcePath, iskills.SkillFileName)
		if _, err := os.Stat(skillFile); err != nil {
			return "", "", fmt.Errorf("sdk.ImportSkill: SKILL.md not found in source directory: %w", err)
		}
		return skillFile, sourcePath, nil
	}

	if filepath.Base(sourcePath) != iskills.SkillFileName {
		return "", "", fmt.Errorf("sdk.ImportSkill: source file must be %s", iskills.SkillFileName)
	}

	return sourcePath, filepath.Dir(sourcePath), nil
}

func ensureSkillNotDuplicate(name string) error {
	existing := append([]*iskills.Skill{}, iskills.DiscoverBuiltin()...)
	existing = append(existing, iskills.Discover(config.GlobalSkillsDirs())...)
	for _, skill := range existing {
		if skill.Name == name {
			return fmt.Errorf("sdk.ImportSkill: duplicate skill name: %s", name)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
