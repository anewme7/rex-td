local terminal = "com.superlogical.terminal"
local links = rex.args.links
local queries = {}
for _, link in ipairs(links) do queries[#queries + 1] = link.block_id end

local resolved, err = rex.call("target.resolve", { kinds = { "block" }, queries = queries })
if not resolved or #resolved.results ~= #links then error(err or "invalid target.resolve reply", 0) end

local function terminal_locations(sid)
  local attached = rex.call("session.attach", { session_id = sid })
  local layout = attached and rex.call("session.view", { session_id = sid })
  if not layout then return false end

  local locations = {}
  for _, window in ipairs(layout.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.creator_name == terminal then
          locations[block.block_id] = {
            block_id = block.block_id, session_id = sid, window_id = window.window_id,
            label = block.label, session = layout.label, window = window.label,
          }
        end
      end
    end
  end
  return locations
end

local locations = {}
local replies = {}
for i, link in ipairs(links) do
  local match = resolved.results[i]
  local target = match.target
  local reply = { block_id = link.block_id, unavailable = "pane_closed" }
  if match.query ~= link.block_id then error("invalid target.resolve query", 0) end
  if match.status == "resolved" and target and target.kind == "block" and
      target.block_id == link.block_id and target.session_id == link.session_id and target.window_id then
    local sid = target.session_id
    if locations[sid] == nil then locations[sid] = terminal_locations(sid) end
    local session = locations[sid]
    local location = session and session[target.block_id]
    if not session then
      reply.unavailable = "host_unavailable"
    elseif location then
      -- The current view supplies labels and verifies that the block is
      -- still placed in this session and is a terminal.
      local status = rex.call(terminal .. ".program_status", {
        session_id = sid, block_id = target.block_id, args = {},
      })
      location.records = status and status.records
      reply = location
    end
  end
  replies[i] = reply
end
return { targets = replies }
