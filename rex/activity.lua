-- The existing sidebar watches one session. Go can watch a whole Rex server
-- with --all fleet=true and consumes the same JSON snapshot format.
local terminal = "com.superlogical.terminal"
local args = rex.args or {}
local fleet = args.fleet == true
local excluded = args.sidebar_id or rex.block_id
local sessions = {}
local layouts, changes, titles = {}, {}, {}
local topology, timer
local attached_all = false

local function read_status(session_id, block_id, pane)
  local status = rex.call(terminal .. ".program_status", {
    session_id = session_id,
    block_id = block_id,
    args = {},
  })
  pane.records = status and status.records
  pane.unavailable = status == nil
end

local function read_title(session_id, block_id, pane)
  local result = rex.call(terminal .. ".title", {
    session_id = session_id, block_id = block_id, args = {},
  })
  pane.terminal_title = result and result.title
end

local function load_layout(id, entry)
  if fleet and not attached_all and not entry.attached then
    local attached = rex.call("session.attach", { session_id = id })
    if not attached then
      entry.unavailable = true
      return
    end
    entry.attached = true
  end
  local view, err = rex.call("session.view", { session_id = id })
  if not view then
    if not fleet then error(err, 0) end
    entry.unavailable = true
    return
  end

  local panes = {}
  for _, window in ipairs(view.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        local block_id = block.block_id
        if block_id ~= excluded and block.creator_name == terminal then
          local old = entry.panes[block_id]
          local pane = old or {}
          pane.label = block.label
          pane.window = window.label
          pane.window_id = window.window_id
          pane.session = view.label
          pane.session_id = id
          if not old or pane.unavailable then read_status(id, block_id, pane) end
          if fleet and not old then read_title(id, block_id, pane) end
          panes[block_id] = pane
        end
      end
    end
  end
  entry.panes = panes
  entry.windows = #(view.windows or {})
  entry.unavailable = false
end

local function load_sessions(list)
  if not list then
    local err
    list, err = rex.call("session.list", {})
    if not list then error(err, 0) end
  end
  local current = {}
  for _, session in ipairs(list.sessions or {}) do
    local id = session.session_id
    local entry = sessions[id]
    if not entry or entry.unavailable then layouts[id] = true end
    current[id] = entry or { panes = {}, windows = 0 }
  end
  sessions = current
end

local function publish()
  local panes, count, windows, partial = {}, 0, 0, false
  for _, session in pairs(sessions) do
    count = count + 1
    windows = windows + session.windows
    partial = partial or session.unavailable == true
    for id, pane in pairs(session.panes) do
      panes[id] = pane
      partial = partial or pane.unavailable
    end
  end
  local payload, err = rex.json.encode({
    panes = panes, partial = partial, session_count = count, window_count = windows,
  })
  if not payload then error(err, 0) end
  print(payload)
end

local function flush()
  timer = nil
  if topology then load_sessions() end
  for id in pairs(layouts) do
    if sessions[id] then load_layout(id, sessions[id]) end
  end
  for block_id, session_id in pairs(changes) do
    local session = sessions[session_id]
    local pane = session and session.panes[block_id]
    if pane then read_status(session_id, block_id, pane) end
  end
  for block_id, session_id in pairs(titles) do
    local session = sessions[session_id]
    local pane = session and session.panes[block_id]
    if pane then read_title(session_id, block_id, pane) end
  end
  topology, layouts, changes, titles = false, {}, {}, {}
  publish()
end

local function schedule()
  if not timer then timer = rex.after(0.08, flush) end
end

rex.on("session_view_changed", function(ctx)
  if ctx.session_id then
    layouts[ctx.session_id] = true
    schedule()
  end
end)
local function descendant(inherited_id, parent_id)
  return inherited_id ~= parent_id and
    (parent_id == "" or inherited_id:sub(1, #parent_id + 1) == parent_id .. "/")
end

local function inherits_app(records, id)
  for _, record in ipairs(records) do
    if not record.app and descendant(record.id or "", id) then return true end
  end
  return false
end

local function update_record(records, ev)
  local record = ev.record
  if type(ev.id) ~= "string" or type(record) ~= "table" or
      type(record.state) ~= "string" or type(record.updated_at) ~= "string" or
      (record.id or "") ~= ev.id then
    return false
  end
  for i, old in ipairs(records) do
    if (old.id or "") == ev.id then
      -- A parent's app change can affect children that did not report.
      if old.app ~= record.app and inherits_app(records, ev.id) then return false end
      records[i] = record
      return true
    end
  end
  if record.app and inherits_app(records, ev.id) then return false end
  records[#records + 1] = record
  return true
end

local function remove_records(records, ev)
  if type(ev.ids) ~= "table" then return false end
  local removed = {}
  for _, id in ipairs(ev.ids) do
    if type(id) ~= "string" then return false end
    removed[id] = true
  end
  if not next(removed) then return false end
  for _, record in ipairs(records) do
    local id = record.id or ""
    if not record.app and not removed[id] then
      for parent in pairs(removed) do
        if descendant(id, parent) then return false end
      end
    end
  end
  for i = #records, 1, -1 do
    if removed[records[i].id or ""] then table.remove(records, i) end
  end
  return true
end

local function record_event(ctx, ev, apply)
  if not ctx.session_id or not ctx.block_id or ctx.block_id == excluded then return end
  local session = sessions[ctx.session_id]
  local pane = session and session.panes[ctx.block_id]
  if not pane then return end
  if pane.unavailable or type(pane.records) ~= "table" or
      (not changes[ctx.block_id] and not apply(pane.records, ev or {})) then
    changes[ctx.block_id] = ctx.session_id
  end
  local record = ev and ev.record
  if type(record) == "table" and type(record.updated_at) == "string" and
      (record.state == "working" or record.state == "blocked" or record.state == "error") then
    pane.activity_at = record.updated_at
  end
  schedule()
end

rex.on("terminal.program_status_changed", function(ctx, ev)
  record_event(ctx, ev, update_record)
end)
rex.on("terminal.program_status_removed", function(ctx, ev)
  record_event(ctx, ev, remove_records)
end)

if fleet then
  rex.on("terminal.title_changed", function(ctx)
    if ctx.session_id and ctx.block_id and ctx.block_id ~= excluded then
      titles[ctx.block_id] = ctx.session_id
      schedule()
    end
  end)
  local function refresh_sessions()
    topology = true
    schedule()
  end
  rex.on("session_created", refresh_sessions)
  rex.on("session_destroyed", refresh_sessions)
  rex.on("session_label_changed", function(ctx)
    if ctx.session_id then
      layouts[ctx.session_id] = true
      schedule()
    end
  end)
  -- --all selects the event scope. Explicitly attach before the first
  -- session.view or terminal method, using the returned session list.
  local initial = rex.call("session.attach_all", { event_scope = "all" })
  attached_all = initial ~= nil
  if attached_all then
    load_sessions(initial)
  else
    topology = true
  end
else
  sessions[rex.session_id] = { panes = {}, windows = 0 }
  layouts[rex.session_id] = true
end
flush()
