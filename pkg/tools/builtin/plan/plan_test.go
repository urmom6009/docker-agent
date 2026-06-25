package plan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/tools"
)

func newTestPlanTool(t *testing.T) *ToolSet {
	t.Helper()
	return New(WithStorage(NewFilesystemStorage(t.TempDir())))
}

// newTestPlanToolWithDir builds a filesystem-backed toolset and returns the
// directory it stores plans in, for tests that need to plant files directly.
func newTestPlanToolWithDir(t *testing.T) (*ToolSet, string) {
	t.Helper()
	dir := t.TempDir()
	return New(WithStorage(NewFilesystemStorage(dir))), dir
}

func TestPlanTool_DisplayNames(t *testing.T) {
	tool := newTestPlanTool(t)

	all, err := tool.Tools(t.Context())
	require.NoError(t, err)

	for _, tl := range all {
		assert.NotEmpty(t, tl.DisplayName())
		assert.NotEqual(t, tl.Name, tl.DisplayName())
	}
}

func TestPlanTool_Instructions(t *testing.T) {
	tool := newTestPlanTool(t)
	assert.NotEmpty(t, tool.Instructions())
}

func TestPlanTool_Describe(t *testing.T) {
	tool := newTestPlanTool(t)
	assert.Contains(t, tool.Describe(), "plan(dir=")
}

func TestPlanTool_DescribeCustomBackend(t *testing.T) {
	// A custom backend that is not a fmt.Stringer falls back to a bare label.
	tool := New(WithStorage(newMemoryStorage()))
	assert.Equal(t, "plan", tool.Describe())
}

func TestPlanTool_Write(t *testing.T) {
	tool := newTestPlanTool(t)

	result, err := tool.writePlan(t.Context(), WritePlanArgs{
		Name:    "release",
		Content: "Step 1: do the thing",
		Title:   "Release plan",
		Author:  "planner",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var plan Plan
	require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
	assert.Equal(t, "release", plan.Name)
	assert.Equal(t, "Release plan", plan.Title)
	assert.Equal(t, "Step 1: do the thing", plan.Content)
	assert.Equal(t, "planner", plan.Author)
	assert.Equal(t, 1, plan.Revision)
	assert.NotEmpty(t, plan.UpdatedAt)
}

func TestPlanTool_WriteEmptyContent(t *testing.T) {
	tool := newTestPlanTool(t)

	result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: ""})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Output, "content must not be empty")
}

func TestPlanTool_InvalidNames(t *testing.T) {
	tool := newTestPlanTool(t)

	for _, name := range []string{"", "///", "Has Space", "UPPER", "../escape", "a/b", "-leading", "with.dot"} {
		result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: name, Content: "x"})
		require.NoError(t, err)
		assert.True(t, result.IsError, "name %q should be rejected", name)
		assert.Contains(t, result.Output, "invalid plan name")
	}
}

func TestPlanTool_ValidNames(t *testing.T) {
	tool := newTestPlanTool(t)

	for _, name := range []string{"release", "release-2025", "db_migration", "a", "1plan"} {
		result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: name, Content: "x"})
		require.NoError(t, err)
		assert.False(t, result.IsError, "name %q should be accepted: %s", name, result.Output)
	}
}

func TestPlanTool_NoSilentCollision(t *testing.T) {
	tool := newTestPlanTool(t)

	// "a-b" is valid; "a/b" and "a b" are rejected outright rather than
	// being silently mapped onto "a-b" and clobbering it.
	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "a-b", Content: "original"})
	require.NoError(t, err)

	for _, colliding := range []string{"a/b", "a b", "a!b"} {
		result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: colliding, Content: "evil"})
		require.NoError(t, err)
		assert.True(t, result.IsError, "name %q must not be accepted", colliding)
	}

	// The original plan is untouched.
	result, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: "a-b"})
	require.NoError(t, err)
	var plan Plan
	require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
	assert.Equal(t, "original", plan.Content)
}

func TestPlanTool_ReadNotFound(t *testing.T) {
	tool := newTestPlanTool(t)

	result, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: "missing"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Output, "not found")
}

func TestPlanTool_ReadCorruptReportsError(t *testing.T) {
	tool, dir := newTestPlanToolWithDir(t)

	// Write a corrupt plan file directly.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o600))

	result, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: "broken"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Output, "corrupt")
	assert.NotContains(t, result.Output, "not found")
}

func TestPlanTool_WriteThenRead(t *testing.T) {
	tool := newTestPlanTool(t)

	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "migration", Content: "the plan", Title: "T"})
	require.NoError(t, err)

	result, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: "migration"})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var plan Plan
	require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
	assert.Equal(t, "the plan", plan.Content)
	assert.Equal(t, "T", plan.Title)
}

func TestPlanTool_RevisionIncrementsAndMetadataPreserved(t *testing.T) {
	tool := newTestPlanTool(t)

	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: "v1", Title: "Original", Author: "alice"})
	require.NoError(t, err)

	// Second write omits the title and author; both should be preserved.
	result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: "v2"})
	require.NoError(t, err)

	var plan Plan
	require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
	assert.Equal(t, "v2", plan.Content)
	assert.Equal(t, "Original", plan.Title)
	assert.Equal(t, "alice", plan.Author)
	assert.Equal(t, 2, plan.Revision)
}

func TestPlanTool_AuthorCanBeUpdated(t *testing.T) {
	tool := newTestPlanTool(t)

	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: "v1", Author: "alice"})
	require.NoError(t, err)

	result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: "v2", Author: "bob"})
	require.NoError(t, err)

	var plan Plan
	require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
	assert.Equal(t, "bob", plan.Author)
}

func TestPlanTool_ListPlans(t *testing.T) {
	tool := newTestPlanTool(t)

	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "beta", Content: "b", Author: "x"})
	require.NoError(t, err)
	_, err = tool.writePlan(t.Context(), WritePlanArgs{Name: "alpha", Content: "a", Author: "y"})
	require.NoError(t, err)

	result, err := tool.listPlans(t.Context(), tools.ToolCall{})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var list ListResult
	require.NoError(t, json.Unmarshal([]byte(result.Output), &list))
	require.Len(t, list.Plans, 2)
	// Sorted by name.
	assert.Equal(t, "alpha", list.Plans[0].Name)
	assert.Equal(t, "beta", list.Plans[1].Name)
	assert.Empty(t, list.Warnings)
}

func TestPlanTool_ListEmpty(t *testing.T) {
	tool := newTestPlanTool(t)

	result, err := tool.listPlans(t.Context(), tools.ToolCall{})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var list ListResult
	require.NoError(t, json.Unmarshal([]byte(result.Output), &list))
	assert.Empty(t, list.Plans)
	assert.Empty(t, list.Warnings)
	// An empty listing serializes as "plans":[] rather than "plans":null.
	assert.Contains(t, result.Output, `"plans":[]`)
}

func TestPlanTool_ListSkipsCorrupt(t *testing.T) {
	tool, dir := newTestPlanToolWithDir(t)

	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "good", Content: "ok"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{nope"), 0o600))

	result, err := tool.listPlans(t.Context(), tools.ToolCall{})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var list ListResult
	require.NoError(t, json.Unmarshal([]byte(result.Output), &list))
	require.Len(t, list.Plans, 1)
	assert.Equal(t, "good", list.Plans[0].Name)
	// The corrupt file is surfaced as a warning rather than silently dropped.
	require.Len(t, list.Warnings, 1)
	assert.Contains(t, list.Warnings[0], "bad")
}

func TestPlanTool_ListNameFromFilename(t *testing.T) {
	tool, dir := newTestPlanToolWithDir(t)

	// A plan file whose stored name field disagrees with its filename. The
	// filename is authoritative because read_plan/delete_plan key off it.
	drifted := Plan{Name: "wrong-name", Content: "x", Revision: 1}
	data, err := json.Marshal(drifted)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real-name.json"), data, 0o600))

	result, err := tool.listPlans(t.Context(), tools.ToolCall{})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var list ListResult
	require.NoError(t, json.Unmarshal([]byte(result.Output), &list))
	require.Len(t, list.Plans, 1)
	// The name returned matches the filename, so a follow-up read_plan works.
	assert.Equal(t, "real-name", list.Plans[0].Name)

	read, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: list.Plans[0].Name})
	require.NoError(t, err)
	assert.False(t, read.IsError)
}

func TestPlanTool_DeletePlan(t *testing.T) {
	tool := newTestPlanTool(t)

	_, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "temp", Content: "x"})
	require.NoError(t, err)

	result, err := tool.deletePlan(t.Context(), DeletePlanArgs{Name: "temp"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Output, "temp")

	// Verify it's gone.
	readResult, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: "temp"})
	require.NoError(t, err)
	assert.True(t, readResult.IsError)
}

func TestPlanTool_DeleteCorruptSucceeds(t *testing.T) {
	tool, dir := newTestPlanToolWithDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{nope"), 0o600))

	result, err := tool.deletePlan(t.Context(), DeletePlanArgs{Name: "broken"})
	require.NoError(t, err)
	assert.False(t, result.IsError, "a corrupt plan should still be deletable")
}

func TestPlanTool_DeleteNotFound(t *testing.T) {
	tool := newTestPlanTool(t)

	result, err := tool.deletePlan(t.Context(), DeletePlanArgs{Name: "nope"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Output, "not found")
}

func TestPlanTool_SharedAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// One agent writes the plan.
	writer := New(WithStorage(NewFilesystemStorage(dir)))
	_, err := writer.writePlan(t.Context(), WritePlanArgs{
		Name:    "collab",
		Content: "collaborative plan",
		Author:  "agent-a",
	})
	require.NoError(t, err)

	// Another agent, sharing the same folder, reads it.
	reader := New(WithStorage(NewFilesystemStorage(dir)))
	result, err := reader.readPlan(t.Context(), ReadPlanArgs{Name: "collab"})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var plan Plan
	require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
	assert.Equal(t, "collaborative plan", plan.Content)
	assert.Equal(t, "agent-a", plan.Author)
}

func TestPlanTool_ParametersAreObjects(t *testing.T) {
	tool := newTestPlanTool(t)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, allTools)

	for _, tl := range allTools {
		if tl.Parameters == nil {
			continue
		}
		m, err := tools.SchemaToMap(tl.Parameters)
		require.NoError(t, err)
		assert.Equal(t, "object", m["type"])
	}
}

// TestPlanTool_WithCustomStorage verifies WithStorage injects a backend that
// the handlers actually write to and read from.
func TestPlanTool_WithCustomStorage(t *testing.T) {
	storage := newMemoryStorage()
	tool := New(WithStorage(storage))

	result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: "v1", Title: "T", Author: "alice"})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// The write landed in the injected backend, not on disk.
	stored, ok, err := storage.Get(t.Context(), "p")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "v1", stored.Content)
	assert.Equal(t, 1, stored.Revision)

	// read_plan reads it back through the toolset.
	read, err := tool.readPlan(t.Context(), ReadPlanArgs{Name: "p"})
	require.NoError(t, err)
	assert.False(t, read.IsError)
	var got Plan
	require.NoError(t, json.Unmarshal([]byte(read.Output), &got))
	assert.Equal(t, "v1", got.Content)

	// delete_plan removes it from the injected backend.
	del, err := tool.deletePlan(t.Context(), DeletePlanArgs{Name: "p"})
	require.NoError(t, err)
	assert.False(t, del.IsError)
	_, ok, err = storage.Get(t.Context(), "p")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPlanTool_WithStorage_NilPanics(t *testing.T) {
	assert.Panics(t, func() {
		WithStorage(nil)
	})
}

// TestPlanTool_ListNilNormalizedToEmptyArray drives list_plans through a
// backend whose List returns a nil slice, proving the handler emits
// "plans":[] rather than "plans":null regardless of the backend.
func TestPlanTool_ListNilNormalizedToEmptyArray(t *testing.T) {
	tool := New(WithStorage(noBumpStorage{}))

	result, err := tool.listPlans(t.Context(), tools.ToolCall{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Output, `"plans":[]`)
	assert.NotContains(t, result.Output, `"plans":null`)
}

// TestPlanTool_StorageErrorsSurfaceAsIsError verifies every handler maps a
// backend error to an IsError result rather than masking it as not-found,
// empty, or success.
func TestPlanTool_StorageErrorsSurfaceAsIsError(t *testing.T) {
	tool := New(WithStorage(errStorage{}))
	ctx := t.Context()

	write, err := tool.writePlan(ctx, WritePlanArgs{Name: "p", Content: "x"})
	require.NoError(t, err)
	assert.True(t, write.IsError)
	assert.Contains(t, write.Output, "backend boom")

	read, err := tool.readPlan(ctx, ReadPlanArgs{Name: "p"})
	require.NoError(t, err)
	assert.True(t, read.IsError)
	assert.Contains(t, read.Output, "backend boom")

	list, err := tool.listPlans(ctx, tools.ToolCall{})
	require.NoError(t, err)
	assert.True(t, list.IsError)
	assert.Contains(t, list.Output, "backend boom")

	del, err := tool.deletePlan(ctx, DeletePlanArgs{Name: "p"})
	require.NoError(t, err)
	assert.True(t, del.IsError)
	assert.Contains(t, del.Output, "backend boom")
}

// TestPlanTool_RevisionOwnedByStorage proves the revision bump lives in
// Storage.Upsert, not the handler: a backend that never bumps leaves the
// revision untouched across repeated writes.
func TestPlanTool_RevisionOwnedByStorage(t *testing.T) {
	tool := New(WithStorage(noBumpStorage{}))

	for range 3 {
		result, err := tool.writePlan(t.Context(), WritePlanArgs{Name: "p", Content: "x"})
		require.NoError(t, err)
		var plan Plan
		require.NoError(t, json.Unmarshal([]byte(result.Output), &plan))
		assert.Equal(t, 0, plan.Revision)
	}
}

// TestStorage_Conformance exercises every Storage method through the interface
// against both the default filesystem backend and a custom in-memory one, so
// the two stay behaviorally equivalent.
func TestStorage_Conformance(t *testing.T) {
	t.Run("filesystem", func(t *testing.T) {
		runStorageConformance(t, NewFilesystemStorage(t.TempDir()))
	})
	t.Run("memory", func(t *testing.T) {
		runStorageConformance(t, newMemoryStorage())
	})
}

func runStorageConformance(t *testing.T, s Storage) {
	t.Helper()
	ctx := t.Context()

	// A missing plan is reported as not-found, not as an error.
	_, ok, err := s.Get(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, ok)

	// An empty store lists nothing without warnings.
	plans, warnings, err := s.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, plans)
	assert.Empty(t, warnings)

	// Upsert creates the plan at revision 1 and stamps a timestamp.
	p, err := s.Upsert(ctx, "release", "v1", "Release", "alice")
	require.NoError(t, err)
	assert.Equal(t, "release", p.Name)
	assert.Equal(t, "v1", p.Content)
	assert.Equal(t, "Release", p.Title)
	assert.Equal(t, "alice", p.Author)
	assert.Equal(t, 1, p.Revision)
	assert.NotEmpty(t, p.UpdatedAt)

	// Get returns exactly what was stored.
	got, ok, err := s.Get(ctx, "release")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, p, got)

	// Upsert bumps the revision and preserves omitted title/author.
	p2, err := s.Upsert(ctx, "release", "v2", "", "")
	require.NoError(t, err)
	assert.Equal(t, 2, p2.Revision)
	assert.Equal(t, "v2", p2.Content)
	assert.Equal(t, "Release", p2.Title)
	assert.Equal(t, "alice", p2.Author)

	// A non-empty author overwrites the previous one.
	p3, err := s.Upsert(ctx, "release", "v3", "", "bob")
	require.NoError(t, err)
	assert.Equal(t, 3, p3.Revision)
	assert.Equal(t, "bob", p3.Author)

	// List reflects every plan, sorted by name.
	_, err = s.Upsert(ctx, "alpha", "a", "", "")
	require.NoError(t, err)
	plans, warnings, err = s.List(ctx)
	require.NoError(t, err)
	require.Len(t, plans, 2)
	assert.Equal(t, "alpha", plans[0].Name)
	assert.Equal(t, "release", plans[1].Name)
	assert.Empty(t, warnings)

	// Delete removes the plan and reports it was deleted.
	deleted, err := s.Delete(ctx, "release")
	require.NoError(t, err)
	assert.True(t, deleted)
	_, ok, err = s.Get(ctx, "release")
	require.NoError(t, err)
	assert.False(t, ok)

	// Deleting again reports not-deleted, without an error.
	deleted, err = s.Delete(ctx, "release")
	require.NoError(t, err)
	assert.False(t, deleted)
}

// memoryStorage is an in-memory Storage used to exercise the toolset through a
// custom backend. It mirrors the filesystem default's contract: Upsert owns the
// revision bump and preserves title/author when omitted.
type memoryStorage struct {
	mu    sync.Mutex
	plans map[string]Plan
}

var _ Storage = (*memoryStorage)(nil)

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{plans: map[string]Plan{}}
}

func (s *memoryStorage) Get(_ context.Context, name string) (Plan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[name]
	return p, ok, nil
}

func (s *memoryStorage) Upsert(_ context.Context, name, content, title, author string) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.plans[name]
	p.Name = name
	p.Content = content
	if title != "" {
		p.Title = title
	}
	if author != "" {
		p.Author = author
	}
	p.Revision++
	p.UpdatedAt = "2024-01-01T00:00:00Z"
	s.plans[name] = p
	return p, nil
}

func (s *memoryStorage) List(_ context.Context) ([]Summary, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Summary, 0, len(s.plans))
	for name, p := range s.plans {
		out = append(out, Summary{Name: name, Title: p.Title, Author: p.Author, Revision: p.Revision, UpdatedAt: p.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil, nil
}

func (s *memoryStorage) Delete(_ context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[name]; !ok {
		return false, nil
	}
	delete(s.plans, name)
	return true, nil
}

// noBumpStorage is a deliberately inert backend whose Upsert never bumps the
// revision, used to prove the toolset handler does not add a bump of its own.
type noBumpStorage struct{}

var _ Storage = noBumpStorage{}

func (noBumpStorage) Get(context.Context, string) (Plan, bool, error) { return Plan{}, false, nil }

func (noBumpStorage) Upsert(_ context.Context, name, content, title, author string) (Plan, error) {
	return Plan{Name: name, Content: content, Title: title, Author: author, Revision: 0}, nil
}

func (noBumpStorage) List(context.Context) ([]Summary, []string, error) { return nil, nil, nil }

func (noBumpStorage) Delete(context.Context, string) (bool, error) { return false, nil }

// errStorage is a backend whose every method fails, used to verify the
// handlers surface a real backend error as an IsError result.
type errStorage struct{}

var (
	_          Storage = errStorage{}
	errBackend         = errors.New("backend boom")
)

func (errStorage) Get(context.Context, string) (Plan, bool, error) {
	return Plan{}, false, errBackend
}

func (errStorage) Upsert(context.Context, string, string, string, string) (Plan, error) {
	return Plan{}, errBackend
}

func (errStorage) List(context.Context) ([]Summary, []string, error) {
	return nil, nil, errBackend
}

func (errStorage) Delete(context.Context, string) (bool, error) {
	return false, errBackend
}
