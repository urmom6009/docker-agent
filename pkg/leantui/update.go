package leantui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/tui/messages"
)

func (m *model) handleKey(ctx context.Context, k key) {
	if m.confirm != nil {
		m.handleConfirmKey(k)
		return
	}

	switch k.typ {
	case keyCtrlC:
		m.handleInterrupt()
	case keyCtrlD:
		if m.editor.isEmpty() {
			m.quit()
		} else {
			m.editor.deleteForward()
		}
	case keyEnter:
		m.handleEnter(ctx)
	case keyAltEnter:
		m.editor.insertNewline()
	case keyTab:
		m.handleTab()
	case keyUp:
		if m.ac.active {
			m.ac.moveUp()
		} else if !m.editor.up(m.width) {
			m.editor.historyPrev()
		}
	case keyDown:
		if m.ac.active {
			m.ac.moveDown()
		} else if !m.editor.down(m.width) {
			m.editor.historyNext()
		}
	case keyLeft:
		m.editor.moveLeft()
	case keyRight:
		m.editor.moveRight()
	case keyWordLeft:
		m.editor.moveWordLeft()
	case keyWordRight:
		m.editor.moveWordRight()
	case keyHome:
		m.editor.moveLineStart()
	case keyEnd:
		m.editor.moveLineEnd()
	case keyBackspace:
		m.editor.backspace()
	case keyDelete:
		m.editor.deleteForward()
	case keyCtrlU:
		m.editor.deleteToLineStart()
	case keyCtrlK:
		m.editor.deleteToLineEnd()
	case keyCtrlW:
		m.editor.deleteWordBack()
	case keyEsc:
		m.ac.dismiss()
	case keyCtrlL:
		m.clearScreen()
	case keyRune, keyPaste:
		m.editor.insert(k.runes)
	}

	m.ac.sync(m.editor.text())
}

func (m *model) handleInterrupt() {
	switch {
	case m.busy:
		if m.runCancel != nil {
			m.runCancel()
		}
		m.queue = nil
		m.addBlock(func(int) []string { return []string{stWarning().Render("⏹ Cancelled")} })
	case !m.editor.isEmpty():
		m.editor.reset()
		m.ac.dismiss()
	default:
		m.quit()
	}
}

func (m *model) handleEnter(ctx context.Context) {
	if m.ac.active {
		if cmd, ok := m.ac.current(); ok {
			m.ac.dismiss()
			m.submit(ctx, "/"+cmd.name)
			return
		}
	}
	m.submit(ctx, m.editor.text())
}

func (m *model) handleTab() {
	if !m.ac.active {
		return
	}
	if cmd, ok := m.ac.current(); ok {
		m.editor.setText("/" + cmd.name + " ")
		m.ac.sync(m.editor.text())
	}
}

func (m *model) submit(ctx context.Context, text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	m.editor.rememberHistory(trimmed)
	m.editor.reset()
	m.ac.dismiss()

	if strings.HasPrefix(trimmed, "/") && m.handleSlash(ctx, trimmed) {
		return
	}

	m.addUserEcho(trimmed)

	if m.app.IsReadOnly() {
		m.addNotice("⚠ ", "This session is read-only.", stWarning())
		return
	}
	m.enqueueOrRun(ctx, trimmed)
}

// handleSlash dispatches a slash command. It returns true when the command was
// fully handled (built-in, skill, or agent command) and false when the input
// should be treated as a normal message.
func (m *model) handleSlash(ctx context.Context, text string) bool {
	name, rest := splitCommand(text)
	switch name {
	case "exit", "quit":
		m.quit()
		return true
	case "new":
		m.app.NewSession()
		m.resetConversation()
		m.addNotice("", "Started a new session.", stMuted())
		m.refreshCommands(ctx)
		return true
	case "clear":
		m.clearScreen()
		return true
	case "help":
		m.commitHelp()
		return true
	case "compact":
		m.addUserEcho(text)
		m.startCompact(ctx, rest)
		return true
	}

	if skillName, task, ok := m.app.SkillCommandFork(ctx, text); ok {
		m.addUserEcho(text)
		m.startSkillFork(ctx, skillName, task)
		return true
	}

	if _, _, ok := m.app.LookupCommand(ctx, text); ok {
		m.addUserEcho(text)
		m.enqueueOrRun(ctx, m.app.ResolveInput(ctx, text))
		return true
	}

	if resolved, err := m.app.ResolveSkillCommand(ctx, text); err == nil && resolved != "" {
		m.addUserEcho(text)
		m.enqueueOrRun(ctx, resolved)
		return true
	}

	return false
}

// enqueueOrRun starts a run immediately when idle, or queues the message to run
// after the current response finishes.
func (m *model) enqueueOrRun(ctx context.Context, message string) {
	if m.app.IsReadOnly() {
		return
	}
	if m.busy {
		m.queue = append(m.queue, message)
		return
	}
	m.startRun(ctx, message, nil)
}

func (m *model) sendFirstMessage(ctx context.Context, msg, attachPath string) {
	var atts []messages.Attachment
	if attachPath != "" {
		if abs, err := filepath.Abs(attachPath); err == nil {
			atts = append(atts, messages.Attachment{Name: filepath.Base(abs), FilePath: abs})
		}
	}

	trimmed := strings.TrimSpace(msg)
	switch {
	case trimmed != "":
		m.addUserEcho(trimmed)
	case len(atts) > 0:
		m.addNotice("", "(attached "+atts[0].Name+")", stMuted())
	default:
		return
	}

	content := msg
	if strings.HasPrefix(trimmed, "/") {
		if resolved := m.app.ResolveInput(ctx, trimmed); resolved != "" {
			content = resolved
		}
	}
	m.startRun(ctx, content, atts)
}

func (m *model) startRun(ctx context.Context, message string, attachments []messages.Attachment) {
	runCtx, cancel := context.WithCancel(ctx)
	m.runCancel = cancel
	m.busy = true
	m.app.Run(runCtx, cancel, message, attachments)
}

func (m *model) startCompact(ctx context.Context, prompt string) {
	runCtx, cancel := context.WithCancel(ctx)
	m.runCancel = cancel
	m.busy = true
	m.app.CompactSession(runCtx, cancel, prompt)
}

func (m *model) startSkillFork(ctx context.Context, name, task string) {
	runCtx, cancel := context.WithCancel(ctx)
	m.runCancel = cancel
	m.busy = true
	m.app.RunSkillFork(runCtx, cancel, name, task, nil)
}

func (m *model) handleEvent(ctx context.Context, ev any) {
	switch e := ev.(type) {
	case *runtime.StreamStartedEvent:
		m.busy = true
	case *runtime.StreamStoppedEvent:
		m.handleStreamStopped(ctx)
	case *runtime.AgentChoiceReasoningEvent:
		m.appendPending(blockReasoning, e.Content)
	case *runtime.AgentChoiceEvent:
		m.appendPending(blockAssistant, e.Content)
	case *runtime.PartialToolCallEvent:
		m.flushPending()
		name := e.ToolCall.Function.Name
		if e.ToolDefinition != nil {
			name = e.ToolDefinition.DisplayName()
		}
		m.upsertTool(e.ToolCall.ID, name, e.ToolCall.Function.Arguments)
	case *runtime.ToolCallEvent:
		m.flushPending()
		m.upsertTool(e.ToolCall.ID, e.ToolDefinition.DisplayName(), e.ToolCall.Function.Arguments)
	case *runtime.ToolCallOutputEvent:
		if tv := m.tools[e.ToolCallID]; tv != nil {
			tv.output += e.Output
		}
	case *runtime.ToolCallResponseEvent:
		m.finishTool(e)
	case *runtime.ToolCallConfirmationEvent:
		cmd, summary := describeToolCall(e.ToolCall.Function.Arguments)
		m.confirm = &confirmState{
			name:    e.ToolDefinition.DisplayName(),
			tool:    e.ToolDefinition.Name,
			command: cmd,
			summary: summary,
		}
	case *runtime.TokenUsageEvent:
		if e.Usage != nil {
			m.status.contextLength = e.Usage.ContextLength
			m.status.contextLimit = e.Usage.ContextLimit
			m.status.tokens = e.Usage.InputTokens + e.Usage.OutputTokens
		}
	case *runtime.AgentInfoEvent:
		m.status.agent = e.AgentName
		if e.Model != "" {
			m.status.model = e.Model
		}
	case *runtime.TeamInfoEvent:
		m.applyTeamInfo(ctx, e)
	case *runtime.SessionCompactionEvent:
		m.handleSessionCompaction(ctx, e)
	case *runtime.ErrorEvent:
		m.flushPending()
		m.addNotice("✗ ", e.Error, stError())
	case *runtime.WarningEvent:
		m.addNotice("⚠ ", e.Message, stWarning())
	case *runtime.ShellOutputEvent:
		output := e.Output
		m.addBlock(func(w int) []string { return renderToolOutput(output, w) })
	case *runtime.AgentSwitchingEvent:
		if e.Switching && e.ToAgent != "" {
			m.addNotice("→ ", "Switching to "+e.ToAgent, stMuted())
		}
	case *runtime.MaxIterationsReachedEvent:
		m.addNotice("⚠ ", "Maximum iterations reached.", stWarning())
	case *runtime.ModelFallbackEvent:
		m.addNotice("⚠ ", "Model "+e.FailedModel+" failed, falling back to "+e.FallbackModel+".", stWarning())
	}
}

func (m *model) handleStreamStopped(ctx context.Context) {
	if m.finishBusy(ctx) {
		return
	}

	if m.app.ShouldExitAfterFirstResponse() {
		m.quit()
	}
}

func (m *model) handleSessionCompaction(ctx context.Context, e *runtime.SessionCompactionEvent) {
	switch e.Status {
	case "started":
		m.busy = true
	case "completed":
		m.finishBusy(ctx)
	}
}

func (m *model) finishBusy(ctx context.Context) bool {
	m.flushPending()
	m.busy = false
	m.runCancel = nil

	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue = m.queue[1:]
		m.startRun(ctx, next, nil)
		return true
	}
	return false
}

func (m *model) appendPending(kind blockKind, content string) {
	if content == "" {
		return
	}
	if m.pending == nil || m.pending.kind != kind {
		m.flushPending()
		m.pending = &pendingBlock{kind: kind}
	}
	m.pending.text.WriteString(content)
}

// flushPending finalizes the in-progress streamed block into the conversation.
func (m *model) flushPending() {
	if m.pending == nil {
		return
	}
	text := m.pending.text.String()
	kind := m.pending.kind
	m.pending = nil

	switch kind {
	case blockReasoning:
		m.addBlock(func(w int) []string { return renderReasoningLines(text, w) })
	case blockAssistant:
		m.addBlock(func(w int) []string { return renderAssistantLines(text, w) })
	}
}

func (m *model) upsertTool(id, name, argsJSON string) {
	tv := m.tools[id]
	if tv == nil {
		tv = &toolView{name: name}
		m.tools[id] = tv
		m.toolOrder = append(m.toolOrder, id)
		m.toolStart[id] = time.Now()
	}
	if name != "" {
		tv.name = name
	}
	tv.command, tv.argsSummary = describeToolCall(argsJSON)
}

func (m *model) finishTool(e *runtime.ToolCallResponseEvent) {
	id := e.ToolCallID
	tv := m.tools[id]
	if tv == nil {
		tv = &toolView{name: e.ToolDefinition.DisplayName()}
		m.toolStart[id] = time.Now()
	}
	tv.done = true
	if e.Result != nil {
		tv.isError = e.Result.IsError
	}
	if strings.TrimSpace(tv.output) == "" {
		tv.output = e.Response
	}
	tv.elapsed = time.Since(m.toolStart[id])

	view := *tv
	m.addBlock(func(w int) []string { return renderTool(view, w) })

	delete(m.tools, id)
	delete(m.toolStart, id)
	m.toolOrder = slices.DeleteFunc(m.toolOrder, func(s string) bool { return s == id })
}

func (m *model) applyTeamInfo(ctx context.Context, e *runtime.TeamInfoEvent) {
	for _, a := range e.AvailableAgents {
		if a.Name != e.CurrentAgent {
			continue
		}
		m.status.agent = a.Name
		switch {
		case a.Provider != "" && a.Model != "":
			m.status.model = a.Provider + "/" + a.Model
		case a.Model != "":
			m.status.model = a.Model
		}
		m.status.thinking = a.Thinking
	}
	m.refreshCommands(ctx)
}

func (m *model) refreshCommands(ctx context.Context) {
	cmds := builtinCommands()
	for name, c := range m.app.CurrentAgentCommands(ctx) {
		if m.disabledCommands[name] {
			continue
		}
		cmds = append(cmds, command{name: name, desc: c.DisplayText(), kind: cmdAgent})
	}
	for _, sk := range m.app.CurrentAgentSkills() {
		cmds = append(cmds, command{name: sk.Name, desc: sk.Description, kind: cmdAgent})
	}
	m.ac.setCommands(cmds)
}

func (m *model) handleConfirmKey(k key) {
	if k.typ == keyEsc {
		m.resolveConfirm(runtime.ResumeReject("rejected by user"))
		return
	}
	if k.typ != keyRune || len(k.runes) == 0 {
		return
	}
	switch k.runes[0] {
	case 'y', 'Y':
		m.resolveConfirm(runtime.ResumeApprove())
	case 'a', 'A':
		m.resolveConfirm(runtime.ResumeApproveTool(m.confirm.tool))
	case 's', 'S':
		m.resolveConfirm(runtime.ResumeApproveSession())
	case 'n', 'N':
		m.resolveConfirm(runtime.ResumeReject("rejected by user"))
	}
}

func (m *model) resolveConfirm(req runtime.ResumeRequest) {
	m.app.Resume(req)
	m.confirm = nil
}

func (m *model) resetConversation() {
	if m.runCancel != nil {
		m.runCancel()
		m.runCancel = nil
	}
	m.pending = nil
	m.tools = make(map[string]*toolView)
	m.toolStart = make(map[string]time.Time)
	m.toolOrder = nil
	m.queue = nil
	m.busy = false
	m.confirm = nil
}

func (m *model) clearScreen() {
	m.r.repaint()
}

func (m *model) quit() {
	if m.runCancel != nil {
		m.runCancel()
	}
	m.quitting = true
}

func (m *model) addUserEcho(text string) {
	m.addBlock(func(w int) []string { return renderUserLines(text, w) })
}

func (m *model) addNotice(prefix, text string, style lipgloss.Style) {
	m.addBlock(func(w int) []string { return renderNoticeLines(prefix, text, w, style) })
}

func (m *model) commitHelp() {
	m.addBlock(func(int) []string {
		return []string{
			stBold().Render("Commands"),
			stMuted().Render("  /new       start a new session"),
			stMuted().Render("  /compact   summarize and compact the conversation"),
			stMuted().Render("  /clear     clear the screen"),
			stMuted().Render("  /help      show this help"),
			stMuted().Render("  /exit      quit"),
			"",
			stBold().Render("Shortcuts"),
			stMuted().Render("  Enter      send             Alt+Enter   insert newline"),
			stMuted().Render("  Up/Down    history           Tab         complete command"),
			stMuted().Render("  Ctrl+C     cancel / quit     Ctrl+W      delete previous word"),
		}
	})
}

func splitCommand(text string) (name, rest string) {
	text = strings.TrimPrefix(strings.TrimSpace(text), "/")
	name, rest, _ = strings.Cut(text, " ")
	return name, strings.TrimSpace(rest)
}
