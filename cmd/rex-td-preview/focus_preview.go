package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"rex-td/rex"
)

const linkPage page = helpPage + 1

var focusHighlight = ink.
	Foreground(lipgloss.AdaptiveColor{Light: "#153529", Dark: "#EBF4EC"}).
	Background(lipgloss.AdaptiveColor{Light: "#DDEBE3", Dark: "#2B4540"})

func (m *model) focusView() string {
	w, h := m.width, m.height
	m.mouseRows, m.mouseResourceTitle = nil, -1
	if w == 0 || h == 0 {
		return ""
	}
	if w < 22 || h < 8 {
		return clip(m.text("请扩大 Rex 侧栏  ·  q 退出"), w)
	}
	divider := rules.Render(strings.Repeat("─", w))
	inner := w - 2
	header := accent.Render(m.text("‹ 返回"))
	action := ""
	switch {
	case m.page == listPage && !m.editing:
		action = accent.Render(m.text("+ 新建"))
		tabsWidth := inner - 8
		header = m.tabs(tabsWidth)
	case m.page == detailPage:
		header = accent.Render(m.text("‹ 任务"))
		action = dim.Render(m.text("m 更多"))
	case m.page == scenePage && !m.scene.editing:
		action = dim.Render(m.text("m 更多"))
		if m.scene.more {
			action = dim.Render(m.text("m 简要"))
		}
	case m.page == linkPage:
		header = accent.Render(m.text("‹ 任务"))
	}
	top := []string{inset(pair(header, action, inner)), divider, ""}
	if m.page == listPage && !m.editing && m.attention {
		toggle := m.text("a 全部窗格")
		if m.allPanes {
			toggle = m.text("a 仅提醒")
		}
		top[2] = inset(pair(dim.Render("v"+m.text(" 按")+m.text(groupingLabels[m.grouping])), dim.Render(toggle), inner))
	} else if m.page == listPage && !m.editing {
		toggle := "A 归档"
		if m.archivedView {
			toggle = "A 返回任务"
		}
		top[2] = inset(pair(dim.Render(m.text("/ 搜索")), dim.Render(m.text(toggle)), inner))
	}
	hints := m.hints(inner)
	if m.page == detailPage {
		shortcuts := [][2]string{{"Esc", m.text("返回")}}
		if t := m.selected(); t != nil && len(t.panes) != 0 {
			shortcuts = append(shortcuts, [2]string{"↵", m.text("跳转")})
		}
		hints = renderShortcuts(inner, append(shortcuts, [2]string{"l", m.text("关联")}))
	}
	if m.page == taskMenuPage {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("返回")}, {"↵", m.text("选择")}})
	}
	if m.page == taskPickPage {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("返回")}, {"↵", m.text("选择")}})
	}
	if m.page == linkPage {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("返回")}, {"↵", m.text("关联")}})
	}
	if m.editing && m.candidateDraft == nil {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("取消")}, {"↵", m.text("保存")}})
	}
	bottom := []string{divider, inset(clip(hints, inner))}
	if m.notice != "" && m.page != scenePage {
		bottom = append([]string{inset(clip(accent.Render(m.notice), inner))}, bottom...)
	}
	if m.page == listPage && !m.editing && h >= 10 {
		if m.focus != 0 {
			res := m.resources(inner, h)
			if len(top)+len(res)+len(bottom)+1 < h {
				bottom = append(append([]string{divider}, res...), bottom...)
				m.mouseResourceTitle = h - len(bottom) + 1
			}
		} else {
			status := m.text("正在连接 Rex…")
			if m.demo {
				status = m.text("演示")
			} else if len(m.feed.Hosts) != 0 && m.feed.HostState("local") == rex.Live {
				status = m.text("Rex 已连接")
			} else if !m.demo && !m.rexContext.Inside() {
				status = m.text("需在 Rex 中启动")
			} else if len(m.feed.Hosts) != 0 && m.feed.HostState("local") == rex.Offline {
				status = m.text("Rex 未连接")
			}
			bottom = append([]string{inset(pair(dim.Render(status), dim.Render(m.text("f 机器与额度")), inner))}, bottom...)
			m.mouseResourceTitle = h - len(bottom)
		}
	}
	m.mouseBodyStart = len(top)
	body := m.body(inner, max(1, h-len(top)-len(bottom)))
	return strings.Join(append(append(top, body...), bottom...), "\n")
}

func (m *model) focusTasks(w int) listView {
	var view listView
	if m.taskCount() == 0 {
		if m.archivedView {
			view.append(dim.Render(m.text("暂无归档任务")), listHit{})
			return view
		}
		view.append("", listHit{})
		view.append(ink.Render(m.text("还没有任务")), listHit{})
		view.append("", listHit{})
		view.append(accent.Render(m.text("+ 新建任务")), listHit{action: "+"})
		view.append(dim.Render(m.text("a 查看 Rex 窗格")), listHit{action: "browse"})
	}
	children := m.childRows()
	for _, group := range m.taskGroups() {
		for _, index := range group.rows {
			t := m.tasks[index]
			current := index == m.taskSelection && !m.onCandidates()
			indent := ""
			if parent := m.parentOf(t); parent != nil && (!m.archivedView || parent.archived) {
				indent = "  "
			}
			width := w - len(indent)
			if current && m.focus == 0 {
				view.active = len(view.lines)
				width -= 2
			}
			for row, line := range m.taskRowWithProgress(index, width, m.progressLabel(t, children[t.id])) {
				hit := listHit{}
				if row == 0 {
					hit = listHit{task: t, taskArrowEnd: len(indent) + 5, taskCheckX: len(indent) + 5}
					if current && m.focus == 0 {
						line += " " + dim.Render("…")
						hit.taskMore = true
					}
				}
				view.append(indent+line, hit)
			}
		}
	}
	if !m.archivedView {
		m.appendCandidateRows(&view, w)
	}
	return view
}

func (m *model) focusSummary(t *task, w int) []string {
	var lines []string
	next := m.taskText(t, t.note)
	if next == "" && t.completed < len(t.steps) {
		next = m.taskText(t, t.steps[t.completed])
	}
	if next != "" {
		lines = append(lines, "    "+dim.Render(clip(m.text("下一步")+" · "+next, w-4)))
	}
	if len(t.panes) > 0 {
		lines = append(lines, "    "+dim.Render(clip(fmt.Sprintf(m.text("%d 个会话 · %d 个窗格"), len(m.associatedSessions(t)), len(t.panes)), w-4)))
	} else {
		lines = append(lines, "    "+dim.Render(m.text("还没有关联窗格")))
	}
	return lines
}

func (m *model) associatedSessions(t *task) []listGroup {
	var groups []listGroup
	for i, p := range t.panes {
		addGroup(&groups, m.sessionKey(m.currentPane(p), true), i)
	}
	return groups
}

func (m *model) orderedAssociations(t *task) []int {
	var order []int
	for _, g := range m.associatedSessions(t) {
		order = append(order, g.rows...)
	}
	return order
}

func (m *model) focusDetails(w int) listView {
	var v listView
	t := m.selected()
	if t == nil {
		return v
	}
	add := func(s string) { v.append(s, listHit{}) }
	children := m.childRows()[t.id]
	state := m.text(stateLabels[t.state])
	if progress := m.progressLabel(t, children); progress != "" {
		state += " · " + progress
	}
	v.append(pair(ink.Render(taskCheckbox(t)+" "+m.taskText(t, t.title)), statusColor(stateLabels[t.state]).Render(state), w),
		listHit{task: t, taskCheckX: 1})
	if parent := m.parentOf(t); parent != nil {
		v.append(dim.Render(m.text("主任务")+" · "+m.taskText(parent, parent.title)), listHit{task: parent})
	}
	next := m.taskText(t, t.note)
	if next == "" && t.completed < len(t.steps) {
		next = m.taskText(t, t.steps[t.completed])
	}
	if next != "" {
		add("")
		add(dim.Render(m.text("下一步")))
		for _, line := range wrap(next, w) {
			add(line)
		}
	}
	if pending, ok := m.urgentTaskPane(t); ok {
		add("")
		v.append(pair(accent.Render(m.text("w 看待处理")), m.status(pending.Status), w), listHit{action: "w"})
		v.append(clip("  "+m.paneDisplayTitle(pending)+" · "+m.hostLabel(pending.Host), w), listHit{action: "w"})
	}
	if len(children) != 0 {
		add("")
		add(dim.Render(m.text("子任务")))
		for _, index := range children {
			child := m.tasks[index]
			label := m.text(stateLabels[child.state])
			if child.archived {
				label = m.text("归档")
			}
			if len(child.panes) == 1 {
				label += " · " + m.hostLabel(child.panes[0].Host)
			}
			v.append(pair("  "+taskCheckbox(child)+" "+m.taskText(child, child.title), dim.Render(label), w),
				listHit{task: child, taskCheckX: 3})
		}
	}
	add("")
	groups := m.associatedSessions(t)
	if len(groups) == 0 {
		add(dim.Render(m.text("还没有关联窗格")))
		v.append(accent.Render(m.text("l 关联 Rex 窗格")), listHit{action: "l"})
	} else {
		add(dim.Render(m.text("关联会话")) + fmt.Sprintf("  %d", len(groups)))
		for _, g := range groups {
			first := m.currentPane(t.panes[g.rows[0]])
			expanded := slices.Contains(g.rows, m.detailSelection)
			marker := "▸ "
			if expanded {
				marker = "▾ "
			}
			v.append(pair(ink.Render(marker+g.key.title), dim.Render(m.hostLabel(first.Host)), w),
				listHit{groupID: g.key.id})
			if !expanded {
				continue
			}
			for _, i := range g.rows {
				p := m.currentPane(t.panes[i])
				status := dim.Render(appName(p.App))
				if reason := m.bindingReason(p); reason != "" {
					status = warn.Render(reason)
				}
				row := pair("  "+m.paneDisplayTitle(p), status, w)
				if i == m.detailSelection {
					v.active = len(v.lines)
					row = focusHighlight.Render(pair("› "+m.paneDisplayTitle(p), status, w))
				}
				v.append(row, listHit{pane: identityOf(p)})
				if p.Window != "" && p.Window != m.paneDisplayTitle(p) && !generatedWindowLabel(p.Window, p.WindowID) {
					add("    " + dim.Render(m.sampleText(p.Window)))
				}
				if i == m.detailSelection && m.bindingReason(p) != "" {
					v.append("    "+accent.Render(m.text("r 重新关联")), listHit{action: "r"})
				}
			}
			add("")
		}
	}
	if len(t.steps) > 0 {
		add(dim.Render(fmt.Sprintf(m.text("子任务 %d/%d"), t.completed, len(t.steps))))
		for i, step := range t.steps {
			check := "○ "
			if i < t.completed {
				check = "✓ "
			}
			add("  " + check + m.taskText(t, step))
		}
	}
	if t.result != "" {
		add("")
		add(dim.Render(m.text("结果")))
		for _, line := range wrap(m.taskText(t, t.result), w) {
			add(line)
		}
	}
	if t.kind != uncategorized {
		add(dim.Render(m.text("t 类型  ") + m.text(kindLabels[t.kind])))
	}
	return v
}

func (m *model) bindingReason(p rex.Pane) string {
	if m.demo {
		return ""
	}
	switch m.feed.HostState(p.Host.ID) {
	case rex.Offline:
		return m.text("主机失联")
	case rex.Live:
		if !slices.ContainsFunc(m.feed.Panes, func(live rex.Pane) bool { return samePane(live, p) }) {
			return m.text("窗格当前不可用")
		}
	}
	return ""
}

func (m *model) pickable() []rex.Pane {
	var out []rex.Pane
	for _, p := range m.feed.Panes {
		if p.ID == m.rexContext.SidebarID || m.feed.HostState(p.Host.ID) == rex.Offline || samePane(p, m.replacePane) {
			continue
		}
		if p.App != "" || m.pickerAll {
			out = append(out, p)
		}
	}
	return out
}

func (m *model) focusPicker(w int) listView {
	var v listView
	v.append(pair(ink.Render(m.text("关联 Rex 窗格")), dim.Render(m.text("a 切换范围")), w), listHit{action: "a"})
	v.append("", listHit{})
	panes := m.pickable()
	if len(panes) == 0 {
		v.append(dim.Render(m.text("没有可关联的窗格")), listHit{})
	}
	for i, pane := range panes {
		row := pair("  "+m.paneDisplayTitle(pane), m.status(pane.Status), w)
		if i == m.pickerSelection {
			v.active = len(v.lines) + 1
			row = focusHighlight.Render(pair("› "+m.paneDisplayTitle(pane), m.text(statusText(pane.Status)), w))
		}
		v.append(row, listHit{pane: identityOf(pane)})
		v.append("    "+dim.Render(m.paneLocation(pane)), listHit{pane: identityOf(pane)})
	}
	return v
}

func (m *model) openFocusPicker(replace bool) {
	if t := m.selected(); t == nil || m.archivedTask(t) {
		m.notice = m.text("请先还原任务")
		return
	}
	m.replacePane = rex.Pane{}
	if replace {
		if t := m.selected(); t != nil && len(t.panes) > 0 {
			m.replacePane = t.panes[clamp(m.detailSelection, 0, len(t.panes)-1)]
		}
	}
	m.notice, m.pickerSelection, m.pickerAll = "", 0, false
	m.page = linkPage
}

func (m *model) focusLinkKey(key string) {
	panes := m.pickable()
	switch key {
	case "esc", "left":
		m.page, m.notice = detailPage, ""
	case "a":
		m.pickerAll, m.pickerSelection = !m.pickerAll, 0
	case "up", "k":
		m.pickerSelection = clamp(m.pickerSelection-1, 0, len(panes)-1)
	case "down", "j":
		m.pickerSelection = clamp(m.pickerSelection+1, 0, len(panes)-1)
	case "enter":
		t := m.selected()
		if t == nil || m.pickerSelection < 0 || m.pickerSelection >= len(panes) {
			return
		}
		pane := panes[m.pickerSelection]
		index := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return samePane(p, m.replacePane) })
		existing := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return samePane(p, pane) })
		if index >= 0 {
			if m.store != nil && !m.saved(m.store.replace(t.id, m.replacePane, pane)) {
				return
			}
			if existing >= 0 {
				t.panes = slices.Delete(t.panes, index, index+1)
			} else {
				t.panes[index] = pane
			}
		} else if existing < 0 {
			if m.store != nil && !m.saved(m.store.bind(t.id, pane, false)) {
				return
			}
			t.panes = append(t.panes, pane)
		}
		m.detailSelection = slices.IndexFunc(t.panes, func(p rex.Pane) bool { return samePane(p, pane) })
		m.page, m.notice = detailPage, m.text("已关联 · ")+m.paneDisplayTitle(pane)
		m.refreshCandidates()
	}
}

func (s *taskStore) replace(task int64, previous, next rex.Pane) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, insertPaneBinding, paneBindingValues(task, next)...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM rex_pane_binding WHERE task=? AND host_id=? AND session_id=? AND block_id=?",
		task, previous.Host.ID, previous.SessionID, previous.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *model) focusBrowse() {
	m.page, m.attention, m.allPanes, m.focus = listPage, true, true, 0
	m.refreshPanes()
}

func (m *model) focusMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress || msg.X < 1 || msg.X >= m.width-1 || msg.Y < 0 || msg.Y >= m.height {
		return nil
	}
	if m.page == scenePage && m.scene.editing {
		if msg.Button == tea.MouseButtonLeft && (msg.Y == 0 || msg.Y == m.height-1) {
			key := tea.KeyEsc
			if msg.Y == m.height-1 && m.shortcutAt(msg.X, 2) == 1 {
				key = tea.KeyEnter
			}
			return m.editPaneAgent(tea.KeyMsg{Type: key})
		}
		return nil
	}
	if m.editing {
		if msg.Button == tea.MouseButtonLeft && msg.Y == m.height-1 {
			key := tea.KeyEsc
			confirm := m.shortcutAt(msg.X, 2) == 1
			if m.candidateDraft != nil {
				confirm = !confirm
			}
			if confirm {
				key = tea.KeyEnter
			}
			m.edit(tea.KeyMsg{Type: key})
		}
		return nil
	}
	at := msg.Y - m.mouseBodyStart
	inBody := at >= 0 && at < len(m.mouseRows)
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if msg.Y >= m.mouseBodyStart && msg.Y < m.height-1 {
			key := "up"
			if msg.Button == tea.MouseButtonWheelDown {
				key = "down"
			}
			return m.listCommand(key)
		}
		return nil
	}
	if msg.Button == tea.MouseButtonRight && inBody && m.page == listPage {
		hit := m.mouseRows[at]
		if m.attention && hit.pane != (paneIdentity{}) {
			m.clickListRow(hit, msg.X)
			return m.listCommand("o")
		}
		if !m.attention && hit.task != nil {
			m.selectTask(hit.task)
			return m.listCommand("m")
		}
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if msg.Y == 0 {
		if m.page != listPage {
			if (m.page == detailPage || m.page == scenePage) && msg.X >= m.width-9 {
				return m.listCommand("m")
			}
			return m.listCommand("esc")
		}
		if msg.X >= m.width-8 {
			return m.listCommand("+")
		}
		m.attention, m.focus, m.peek = msg.X >= 1+(m.width-10)/2, 0, false
		m.candidatesFocused = false
		return nil
	}
	if msg.Y == m.height-1 {
		switch m.page {
		case detailPage:
			if t := m.selected(); t != nil && len(t.panes) != 0 {
				return m.listCommand([...]string{"esc", "enter", "l"}[m.shortcutAt(msg.X, 3)])
			}
			return m.listCommand([...]string{"esc", "l"}[m.shortcutAt(msg.X, 2)])
		case taskMenuPage:
			if m.shortcutAt(msg.X, 2) == 1 {
				return m.listCommand("enter")
			}
			return m.listCommand("esc")
		case taskPickPage:
			if m.shortcutAt(msg.X, 2) == 1 {
				m.confirmTaskPick()
			} else {
				m.page, m.taskPick = m.taskPick.back, nil
			}
			return nil
		case linkPage:
			return m.listCommand([...]string{"esc", "enter"}[m.shortcutAt(msg.X, 2)])
		case scenePage:
			return m.listCommand([...]string{"enter", "d", "w", "esc"}[m.shortcutAt(msg.X, 4)])
		case listPage:
			if m.focus != 0 {
				return m.listCommand("esc")
			}
			if m.onCandidates() {
				if m.candidateIndex() >= 0 {
					return m.listCommand([...]string{"enter", "i", "left"}[m.shortcutAt(msg.X, 3)])
				}
				return m.listCommand([...]string{"enter", "up"}[m.shortcutAt(msg.X, 2)])
			}
			return m.listCommand([...]string{"right", "enter", " ", "p"}[m.shortcutAt(msg.X, 4)])
		default:
			return m.listCommand("esc")
		}
	}
	if msg.Y == m.mouseResourceTitle {
		return m.listCommand("f")
	}
	if msg.Y == 2 && m.page == listPage && !m.attention {
		switch {
		case msg.X <= 10:
			return m.listCommand("/")
		case msg.X >= m.width-14:
			return m.listCommand("A")
		}
	}
	if msg.Y == 2 && m.page == listPage && m.attention {
		if msg.X >= m.width/2 {
			return m.listCommand("a")
		}
		return m.listCommand("v")
	}
	if !inBody {
		return nil
	}
	hit := m.mouseRows[at]
	if hit.action == "browse" {
		m.focusBrowse()
		return nil
	}
	if hit.action != "" {
		return m.listCommand(hit.action)
	}
	switch m.page {
	case listPage:
		if hit.task != nil {
			i := slices.Index(m.tasks, hit.task)
			if hit.taskMore && msg.X == m.width-2 {
				m.selectTask(hit.task)
				return m.listCommand("m")
			}
			if i >= 0 && msg.X == hit.taskCheckX {
				m.taskSelection, m.candidatesFocused = i, false
				return m.listCommand("x")
			}
			if i >= 0 && i == m.taskSelection && !m.onCandidates() {
				return m.listCommand("right")
			}
		}
		m.clickListRow(hit, msg.X)
	case detailPage:
		if hit.task != nil {
			if hit.taskCheckX != 0 && msg.X == hit.taskCheckX {
				m.toggleTaskCheck(hit.task)
			} else if hit.task != m.selected() {
				m.selectTask(hit.task)
				m.showDetails()
			}
			return nil
		}
		if t := m.selected(); t != nil {
			if hit.groupID != "" {
				for _, group := range m.associatedSessions(t) {
					if group.key.id == hit.groupID {
						m.detailSelection = group.rows[0]
						break
					}
				}
			} else if i := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return identityOf(p) == hit.pane }); i >= 0 {
				if m.detailSelection == i {
					return m.listCommand("enter")
				}
				m.detailSelection = i
			}
		}
	case taskPickPage:
		if hit.task != nil {
			m.chooseTask(hit.task)
		}
	case linkPage:
		if i := slices.IndexFunc(m.pickable(), func(p rex.Pane) bool { return identityOf(p) == hit.pane }); i >= 0 {
			if i == m.pickerSelection {
				m.focusLinkKey("enter")
			} else {
				m.pickerSelection = i
			}
		}
	}
	return nil
}
