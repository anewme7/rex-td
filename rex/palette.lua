local home = os.getenv("HOME")
local runtime = rextd_root or (home .. "/.local/opt/rextd")
local binary = runtime .. "/rextd"
local language = rextd_language or os.getenv("REX_TD_LANG")
local label = "rextd"
local terminal = "com.superlogical.terminal"
local data = rextd_data_dir or os.getenv("REX_TD_DATA_DIR")
local environment = { "/usr/bin/env", "REX_TD_LANG=" .. (language == "zh" and "zh" or "en") }
if data then environment[#environment + 1] = "REX_TD_DATA_DIR=" .. data end

local function text(en, zh)
  return language == "zh" and zh or en
end

local function command(...)
  local argv = {}
  for _, value in ipairs(environment) do argv[#argv + 1] = value end
  argv[#argv + 1] = binary
  for _, value in ipairs({ ... }) do argv[#argv + 1] = value end
  return argv
end

local states = {
  ["To do"] = "待做", ["In progress"] = "进行中", Blocked = "受阻", Done = "完成", Cancelled = "取消",
}

local function must(value, problem)
  if value == nil then error(problem or text("Rex returned no result", "Rex 未返回结果"), 0) end
  return value
end

local function store(operation, request)
  local argv = command("palette", operation)
  if request then argv[#argv + 1] = rex.json.encode(request) end
  local result = must(rex.exec(argv, { timeout = 5 }))
  if not result.ok then
    error((result.stderr or ""):gsub("%s+$", ""), 0)
  end
  return must(rex.json.decode(result.stdout))
end

local function by_block(view, id)
  if not view or not id then return nil end
  for _, window in ipairs(view.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.block_id == id then return window, block end
      end
    end
  end
end

local function view_of(session_id)
  if not session_id then return nil end
  return rex.call("session.view", { session_id = session_id })
end

local function find_in(view)
  if not view then return nil end
  for _, window in ipairs(view.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.label == label and block.creator_name == terminal then
          local status = rex.call(terminal .. ".process", { session_id = view.session_id, block_id = block.block_id, args = {} })
          -- On macOS Rex tracks login as the child of a directly launched terminal.
          if status and not status.last_exit and
              ((status.foreground and status.foreground.path == binary) or
               (status.child and (status.child.path == binary or status.child.path == "/usr/bin/login"))) then
            return { session_id = view.session_id, window_id = window.window_id, block_id = block.block_id }
          end
        end
      end
    end
  end
end

local function focus(ctx, target, select_window)
  local origin = by_block(view_of(ctx.session_id), ctx.block_id)
  must(rex.call("session.focus_block", { session_id = target.session_id, block_id = target.block_id }))
  if ctx.client_id and (select_window or ctx.session_id ~= target.session_id or not origin or origin.window_id ~= target.window_id) then
    rex.client.queue("session.select", { session_id = target.session_id, window_id = target.window_id })
  end
  return target
end

local function open_board(ctx, args)
  local location = find_in(view_of(ctx.session_id))
  if not location then
    local sessions = must(rex.call("session.list", {}))
    for _, session in ipairs(sessions.sessions or {}) do
      if session.session_id ~= ctx.session_id then
        location = find_in(view_of(session.session_id))
        if location then break end
      end
    end
  end
  if location then return focus(ctx, location, args and args.select_window) end
  if not ctx.session_id or not ctx.block_id then error(text("Run this action from a Rex pane", "请在 Rex 窗格中运行"), 0) end
  local opened = must(rex.call("session.new_split", {
    session_id = ctx.session_id, anchor_id = ctx.block_id,
    direction = "horizontal", side = "after", ratio = 0.65, focus = true,
    layout = {
      block = {
        flavor = terminal .. ".shell", label = label,
        options = { shell = "none", command = command() },
      },
    },
  }))
  return { block_id = opened.block_ids[1], session_id = ctx.session_id }
end

local function task_items(tasks)
  local items, indexed = {}, {}
  for _, task in ipairs(tasks) do
    local id = tostring(task.id)
    items[#items + 1] = { id = id, title = task.title, subtitle = text(task.state, states[task.state] or task.state),
      detail = text(tostring(#task.panes) .. (#task.panes == 1 and " pane" or " panes"), #task.panes .. " 个窗格"), icon = "document" }
    indexed[id] = task
  end
  return items, indexed
end

local function live_targets(task)
  local out, cached = {}, {}
  for _, pane in ipairs(task.panes) do
    if pane.host_id == "local" then
      if cached[pane.session_id] == nil then cached[pane.session_id] = view_of(pane.session_id) or false end
      local view = cached[pane.session_id]
      local window, block = by_block(view, pane.block_id)
      if window and block.creator_name == terminal then
        out[#out + 1] = {
          session_id = pane.session_id, window_id = window.window_id, block_id = pane.block_id,
          title = block.label ~= "" and block.label or pane.title,
          location = (view.label or "Rex") .. " · " .. (window.label or ""),
        }
      end
    end
  end
  return out
end

local function find_task(ctx, args)
  local items, indexed = task_items(store("list"))
  if args.inspect and not args.task_id then return { items = items } end
  if #items == 0 then return open_board(ctx, args) end
  local id = args.task_id and tostring(args.task_id) or rex.ui.pick{
    title = text("Find Task", "查找任务"), placeholder = text("Task name", "任务名称"), items = items,
  }
  if not id then return end
  local task = indexed[id]
  if not task then error(text("The task is no longer available", "任务已不存在，请重新查找"), 0) end
  local targets = live_targets(task)
  if args.inspect then return { title = task.title, targets = targets } end
  if #targets == 0 then
    local opened = open_board(ctx, args)
    opened.task_id = task.id
    return opened
  end
  local chosen
  if args.block_id then
    for _, target in ipairs(targets) do
      if target.block_id == args.block_id then chosen = target end
    end
    if not chosen then error(text("The selected pane is no longer available", "所选窗格已不可用"), 0) end
  elseif #targets == 1 then
    chosen = targets[1]
  else
    local options, by_id = {}, {}
    for _, target in ipairs(targets) do
      options[#options + 1] = { id = target.block_id, title = target.title, subtitle = target.location, icon = "terminal" }
      by_id[target.block_id] = target
    end
    local block_id = rex.ui.pick{ title = task.title, placeholder = text("Choose a pane", "选择一个窗格"), items = options }
    if not block_id then return end
    chosen = by_id[block_id]
  end
  local result = focus(ctx, chosen, args.select_window)
  result.task_id = task.id
  return result
end

local function current_target(ctx)
  local view = view_of(ctx.session_id)
  local window, block = by_block(view, ctx.block_id)
  if not window or block.creator_name ~= terminal or block.label == label then
    error(text("Run this action from the pane you want to link", "请在要收录的工作窗格里运行"), 0)
  end
  local report = rex.call(terminal .. ".program_status", { session_id = ctx.session_id, block_id = ctx.block_id, args = {} })
  local latest
  for _, row in ipairs((report and report.records) or {}) do
    if (row.effective_app or row.app) and (not latest or row.updated_at > latest.updated_at) then latest = row end
  end
  local agent = latest and (latest.effective_app or latest.app) or ""
  local title = block.label
  if not title or title == "" then title = latest and latest.title or "" end
  if title == "" then title = window.label or "" end
  return {
    host_id = "local", host = text("Local", "本机"),
    session_id = ctx.session_id, session = view.label or "", window_id = window.window_id,
    block_id = ctx.block_id, title = title, agent = agent,
  }
end

local function capture(ctx, args)
  local pane = current_target(ctx)
  if args.inspect then return { title = pane.title } end
  local id, title = args.task_id, args.title
  if not id and not title then
    local entries = task_items(store("list"))
    table.insert(entries, 1, { id = "new", title = text("New Task…", "新建任务…"), icon = "plus" })
    local choice = rex.ui.pick{ title = text("Capture Current Pane", "收录当前窗格"), placeholder = text("Choose a task or create one", "选择现有任务或新建"), items = entries }
    if not choice then return end
    if choice == "new" then
      title = rex.ui.input{ title = text("Task title", "任务标题"), value = pane.title, confirm = text("Create and link", "新建并关联") }
      if not title then return end
    else
      id = tonumber(choice)
    end
  end
  pane = current_target(ctx)
  return store("capture", { task_id = id, title = title, pane = pane })
end

rex.action{
  name = "rextd_preview_open",
  title = text("rextd / Open Task Board", "rextd / 打开任务看板"),
  category = "rextd",
  keywords = "tasks board focus 任务 看板 回到侧栏",
  description = text("Open or focus the task board.", "打开或聚焦右侧任务看板。"),
  args = { select_window = "boolean?" },
  run = open_board,
}

rex.action{
  name = "rextd_preview_find",
  title = text("rextd / Find Task…", "rextd / 查找任务…"),
  category = "rextd",
  keywords = "task find search jump 找任务 跳转",
  description = text("Find a task and jump to a linked pane on this Rex host.", "按任务名称找到已关联的本机窗格。"),
  args = { task_id = "integer?", block_id = "string?", inspect = "boolean?", select_window = "boolean?" },
  run = find_task,
}

rex.action{
  name = "rextd_preview_capture",
  title = text("rextd / Capture Current Pane…", "rextd / 收录当前窗格…"),
  category = "rextd",
  keywords = "task capture link 新建 任务 关联 收录",
  description = text("Link the current pane to a task.", "把当前窗格添到任务库。"),
  args = { task_id = "integer?", title = "string?", inspect = "boolean?" },
  run = capture,
}
