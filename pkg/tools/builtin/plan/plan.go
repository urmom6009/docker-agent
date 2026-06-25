// Package plan provides a toolset that lets two or more agents collaborate on
// plans addressed by name. Plans persist through a pluggable Storage backend;
// the default FilesystemStorage keeps them as JSON files in a global shared
// folder under the docker-agent data directory, so any agent that loads this
// toolset can write, read, list, and delete the same shared plans, and they
// persist across sessions. Embedders can inject an alternative backend (e.g. a
// database or remote store) with WithStorage.
//
// Concurrency: agents that share one ToolSet instance also share its Storage,
// which serializes their operations. The default FilesystemStorage guards its
// read-modify-write revision bump with a mutex and writes atomically
// (write-to-temp + rename), so a reader — including a separate docker-agent
// process — never observes a partially written plan. Two distinct processes
// writing the *same* plan at the very same instant can still race on the
// revision bump (last writer wins); this is acceptable for the intended
// in-process multi-agent collaboration. Other backends can make the bump
// atomic.
package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker-agent/pkg/atomicfile"
	"github.com/docker/docker-agent/pkg/paths"
	"github.com/docker/docker-agent/pkg/tools"
)

const (
	ToolNameWritePlan  = "write_plan"
	ToolNameReadPlan   = "read_plan"
	ToolNameListPlans  = "list_plans"
	ToolNameDeletePlan = "delete_plan"
)

// namePattern defines the accepted plan-name format: a lowercase slug made of
// alphanumerics, '-' and '_'. Names are validated against it rather than being
// silently rewritten, so two different inputs can never collapse onto the same
// file (which would let one plan clobber another) and no input can escape the
// plans directory via path separators or "..".
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Plan is a shared document collaborated on by the agents.
type Plan struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
	// Author is a free-form label identifying who last wrote the plan
	// (typically the agent name). It helps collaborators see who made the
	// most recent change.
	Author    string `json:"author,omitempty"`
	Revision  int    `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
}

// Summary is a lightweight view of a plan returned by list_plans.
type Summary struct {
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	Author    string `json:"author,omitempty"`
	Revision  int    `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
}

// ListResult is the output of list_plans. Warnings lists plan files that could
// not be read or decoded, so a caller can tell "no plans exist" apart from
// "some plans failed to load" — important because an agent that mistakes a
// temporarily unreadable plan for a missing one could recreate and clobber it.
type ListResult struct {
	Plans    []Summary `json:"plans"`
	Warnings []string  `json:"warnings,omitempty"`
}

// Storage persists the plans a ToolSet operates on. Implementations decide how
// a plan is stored (files, memory, a database, a remote service) and own the
// revision bump in Upsert, so a backend can make it atomic. The default
// FilesystemStorage is used when WithStorage injects nothing else.
type Storage interface {
	// Get returns the named plan. The bool is false with a nil error when no
	// such plan exists, so callers can tell a missing plan apart from a real
	// read failure (returned as a non-nil error).
	Get(ctx context.Context, name string) (Plan, bool, error)
	// Upsert creates or replaces the named plan's content and, when non-empty,
	// its title and author; omitted title/author are preserved from the
	// previous revision. It bumps the revision and stamps UpdatedAt, returning
	// the stored plan.
	Upsert(ctx context.Context, name, content, title, author string) (Plan, error)
	// List returns a summary of every stored plan. Warnings carries entries
	// that could not be read, so a caller can tell "no plans" apart from "some
	// plans failed to load".
	List(ctx context.Context) (plans []Summary, warnings []string, err error)
	// Delete removes the named plan. The bool is false with a nil error when
	// there was no such plan to delete.
	Delete(ctx context.Context, name string) (deleted bool, err error)
}

type ToolSet struct {
	storage Storage
}

var (
	_ tools.ToolSet      = (*ToolSet)(nil)
	_ tools.Instructable = (*ToolSet)(nil)
	_ tools.Describer    = (*ToolSet)(nil)
)

// Option configures a ToolSet.
type Option func(*ToolSet)

// WithStorage injects a custom Storage backend in place of the default
// FilesystemStorage, letting embedders supply their own store and get
// per-instance isolation. The provided storage must not be nil.
func WithStorage(storage Storage) Option {
	if storage == nil {
		panic("plan: storage must not be nil")
	}
	return func(t *ToolSet) {
		t.storage = storage
	}
}

// sharedToolSet returns the one ToolSet shared by every agent in this process,
// built once on first use. Sharing a single instance means all collaborating
// agents serialize their plan operations on the same storage.
//
// Building the struct cannot fail, so the directory is *not* created here:
// FilesystemStorage runs os.MkdirAll on every write, which means a directory
// that is momentarily unavailable at startup (e.g. a not-yet-mounted parent) is
// recovered from automatically instead of being permanently memoized as an
// error.
var sharedToolSet = sync.OnceValue(func() *ToolSet {
	return New()
})

// CreateToolSet is used by the tools registry. It returns a process-wide
// singleton so that all agents collaborating in the same process share one
// storage over the global plans folder.
func CreateToolSet() (tools.ToolSet, error) {
	return sharedToolSet(), nil
}

// DefaultDir is the global shared folder where plans are stored, under the
// docker-agent data directory.
func DefaultDir() string {
	return filepath.Join(paths.GetDataDir(), "plans")
}

// New builds a per-instance plan toolset. With no options it uses the default
// FilesystemStorage rooted at DefaultDir(); pass WithStorage to inject another
// backend. Each call returns an independent instance — use CreateToolSet for
// the process-wide singleton shared by collaborating agents.
func New(opts ...Option) *ToolSet {
	t := &ToolSet{
		storage: NewFilesystemStorage(DefaultDir()),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *ToolSet) Describe() string {
	if s, ok := t.storage.(fmt.Stringer); ok {
		return "plan(" + s.String() + ")"
	}
	return "plan"
}

func (t *ToolSet) Instructions() string {
	return `## Plan Tools

Collaborate on shared, named plans with other agents. Plans are stored in a
global shared folder, so every agent using this toolset sees the same plans.

- Use list_plans to discover existing plans.
- Use read_plan to inspect a plan before acting on or changing it.
- Use write_plan to create or update a plan by name. Writing replaces the whole
  document, so read it first and preserve any content you want to keep. Set the
  title and author (your agent name) so collaborators can see who made the
  latest change. Each write bumps the revision number.
- Use delete_plan to remove a plan once it is no longer needed.

Plan names must be lowercase and may contain only letters, digits, '-' and '_'
(for example "release-2025" or "db_migration").`
}

type WritePlanArgs struct {
	Name    string `json:"name" jsonschema:"The plan name. Lowercase letters, digits, '-' and '_' only (e.g. 'release', 'db-migration')."`
	Content string `json:"content" jsonschema:"The full plan content (markdown). Replaces the existing plan."`
	Title   string `json:"title,omitempty" jsonschema:"Optional human-readable title. Preserved from the previous revision when omitted."`
	Author  string `json:"author,omitempty" jsonschema:"Optional label identifying who is writing the plan (typically the agent name). Preserved from the previous revision when omitted."`
}

func (t *ToolSet) writePlan(ctx context.Context, params WritePlanArgs) (*tools.ToolCallResult, error) {
	if params.Content == "" {
		return tools.ResultError("content must not be empty"), nil
	}

	plan, err := t.storage.Upsert(ctx, params.Name, params.Content, params.Title, params.Author)
	if err != nil {
		return tools.ResultError(err.Error()), nil
	}

	return tools.ResultJSON(plan), nil
}

type ReadPlanArgs struct {
	Name string `json:"name" jsonschema:"The name of the plan to read."`
}

func (t *ToolSet) readPlan(ctx context.Context, params ReadPlanArgs) (*tools.ToolCallResult, error) {
	plan, ok, err := t.storage.Get(ctx, params.Name)
	if err != nil {
		return tools.ResultError(err.Error()), nil
	}
	if !ok {
		return tools.ResultError(fmt.Sprintf("plan %q not found", params.Name)), nil
	}

	return tools.ResultJSON(plan), nil
}

func (t *ToolSet) listPlans(ctx context.Context, _ tools.ToolCall) (*tools.ToolCallResult, error) {
	plans, warnings, err := t.storage.List(ctx)
	if err != nil {
		return tools.ResultError(err.Error()), nil
	}
	// Always emit a non-nil slice so the output is "plans":[] rather than
	// "plans":null, regardless of what a backend returns when empty.
	if plans == nil {
		plans = []Summary{}
	}

	return tools.ResultJSON(ListResult{Plans: plans, Warnings: warnings}), nil
}

type DeletePlanArgs struct {
	Name string `json:"name" jsonschema:"The name of the plan to delete."`
}

func (t *ToolSet) deletePlan(ctx context.Context, params DeletePlanArgs) (*tools.ToolCallResult, error) {
	deleted, err := t.storage.Delete(ctx, params.Name)
	if err != nil {
		return tools.ResultError(err.Error()), nil
	}
	if !deleted {
		return tools.ResultError(fmt.Sprintf("plan %q not found", params.Name)), nil
	}

	return tools.ResultJSON(map[string]string{"deleted": params.Name}), nil
}

func (t *ToolSet) Tools(context.Context) ([]tools.Tool, error) {
	return []tools.Tool{
		{
			Name:         ToolNameWritePlan,
			Category:     "plan",
			Description:  "Create or update a shared plan by name. Replaces the entire plan content, so read it first to preserve anything you want to keep. Each write bumps the revision number.",
			Parameters:   tools.MustSchemaFor[WritePlanArgs](),
			OutputSchema: tools.MustSchemaFor[Plan](),
			Handler:      tools.NewHandler(t.writePlan),
			Annotations: tools.ToolAnnotations{
				Title: "Write Plan",
			},
		},
		{
			Name:         ToolNameReadPlan,
			Category:     "plan",
			Description:  "Read a shared plan by name, including its title, content, author, and revision number.",
			Parameters:   tools.MustSchemaFor[ReadPlanArgs](),
			OutputSchema: tools.MustSchemaFor[Plan](),
			Handler:      tools.NewHandler(t.readPlan),
			Annotations: tools.ToolAnnotations{
				Title:        "Read Plan",
				ReadOnlyHint: true,
			},
		},
		{
			Name:         ToolNameListPlans,
			Category:     "plan",
			Description:  "List all shared plans with their name, title, author, and revision.",
			OutputSchema: tools.MustSchemaFor[ListResult](),
			Handler:      t.listPlans,
			Annotations: tools.ToolAnnotations{
				Title:        "List Plans",
				ReadOnlyHint: true,
			},
		},
		{
			Name:        ToolNameDeletePlan,
			Category:    "plan",
			Description: "Delete a shared plan by name.",
			Parameters:  tools.MustSchemaFor[DeletePlanArgs](),
			Handler:     tools.NewHandler(t.deletePlan),
			Annotations: tools.ToolAnnotations{
				Title:           "Delete Plan",
				DestructiveHint: new(true),
			},
		},
	}, nil
}

// FilesystemStorage is the default Storage. It persists each plan as a JSON
// file named <name>.json in a directory, with atomic temp+rename writes, plan
// name validation, and unreadable-file warnings on List. A mutex serializes its
// operations so the read-modify-write revision bump in Upsert is consistent
// within a process.
type FilesystemStorage struct {
	mu  sync.Mutex
	dir string
}

var _ Storage = (*FilesystemStorage)(nil)

// NewFilesystemStorage returns a filesystem-backed Storage rooted at dir. The
// directory is created lazily on the first write, not here, so a parent that is
// momentarily unavailable at startup is recovered from automatically.
func NewFilesystemStorage(dir string) *FilesystemStorage {
	return &FilesystemStorage{dir: dir}
}

// String renders the backend for ToolSet.Describe, e.g. "dir=/path/to/plans".
func (s *FilesystemStorage) String() string {
	return "dir=" + s.dir
}

// planPath validates name and returns the absolute path of its plan file. The
// name is rejected (rather than rewritten) when it does not match namePattern,
// which guarantees a one-to-one mapping between names and files and prevents
// path traversal.
func (s *FilesystemStorage) planPath(name string) (string, error) {
	if !namePattern.MatchString(name) {
		return "", fmt.Errorf("invalid plan name %q: use only lowercase letters, digits, '-' and '_', starting with a letter or digit", name)
	}
	return filepath.Join(s.dir, name+".json"), nil
}

// load reads and decodes the plan at path. It distinguishes a missing plan
// (false, nil) from a real failure such as a permission error or corrupt JSON
// (false, err), so callers can report the latter instead of masking it as
// "not found".
func (s *FilesystemStorage) load(path string) (Plan, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Plan{}, false, nil
	}
	if err != nil {
		return Plan{}, false, fmt.Errorf("reading plan: %w", err)
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, false, fmt.Errorf("plan file %s is corrupt: %w", filepath.Base(path), err)
	}
	return p, true, nil
}

func (s *FilesystemStorage) save(path string, p Plan) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("creating plans directory: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plan: %w", err)
	}
	// Atomic write (temp file + rename): readers in other agents or processes
	// see either the old or the new content, never a partial file, and an
	// existing symlink at path is replaced rather than followed.
	if err := atomicfile.Write(path, bytes.NewReader(data), 0o600); err != nil {
		return fmt.Errorf("writing plan: %w", err)
	}
	return nil
}

func (s *FilesystemStorage) Get(_ context.Context, name string) (Plan, bool, error) {
	path, err := s.planPath(name)
	if err != nil {
		return Plan{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load(path)
}

func (s *FilesystemStorage) Upsert(_ context.Context, name, content, title, author string) (Plan, error) {
	path, err := s.planPath(name)
	if err != nil {
		return Plan{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	plan, _, err := s.load(path)
	if err != nil {
		return Plan{}, err
	}
	plan.Name = name
	plan.Content = content
	// Title and author are preserved across revisions when omitted, so an
	// agent updating only the content does not wipe the collaboration metadata
	// set by a previous writer.
	if title != "" {
		plan.Title = title
	}
	if author != "" {
		plan.Author = author
	}
	plan.Revision++
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := s.save(path, plan); err != nil {
		return Plan{}, err
	}

	return plan, nil
}

func (s *FilesystemStorage) List(_ context.Context) ([]Summary, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Summary{}, nil, nil
		}
		return nil, nil, err
	}

	plans := []Summary{}
	var warnings []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		// The filename is the authoritative key: read_plan and delete_plan
		// resolve a name to <name>.json, so the listed name must match the
		// filename, not the (possibly drifted) name field inside the JSON.
		name := strings.TrimSuffix(entry.Name(), ".json")
		plan, ok, err := s.load(filepath.Join(s.dir, entry.Name()))
		if err != nil || !ok {
			// Don't abort the whole listing on one bad file, but surface it as
			// a warning so the caller doesn't mistake an unreadable plan for a
			// missing one.
			msg := "unreadable"
			if err != nil {
				msg = err.Error()
			}
			warnings = append(warnings, fmt.Sprintf("skipped %q: %s", name, msg))
			continue
		}
		plans = append(plans, Summary{
			Name:      name,
			Title:     plan.Title,
			Author:    plan.Author,
			Revision:  plan.Revision,
			UpdatedAt: plan.UpdatedAt,
		})
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Name < plans[j].Name
	})

	return plans, warnings, nil
}

func (s *FilesystemStorage) Delete(_ context.Context, name string) (bool, error) {
	path, err := s.planPath(name)
	if err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove the file directly and treat a missing file as "not deleted". We
	// don't pre-load the plan: a corrupt plan should still be deletable.
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
