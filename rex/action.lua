local terminal = "com.superlogical.terminal"
local chinese = rextd_language == "zh" or (rextd_language == nil and os.getenv("REX_TD_LANG") == "zh")

local function text(english, translated)
  return chinese and translated or english
end

local function sidebar_in_window(ctx, view)
  for _, window in ipairs(view.windows or {}) do
    local candidates, contains_origin = {}, false
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        contains_origin = contains_origin or block.block_id == ctx.block_id
        if block.creator_name == terminal then
          candidates[#candidates + 1] = block
        end
      end
    end
    if contains_origin or (not ctx.block_id and window.window_id == view.active_window_id) then
      for _, block in ipairs(candidates) do
        local id = block.block_id
        local process = rex.call(terminal .. ".process", { session_id = ctx.session_id, block_id = id, args = {} })
        local foreground = process and process.foreground
        local path = foreground and foreground.path
        -- Rex can report the macOS login wrapper for a directly launched command.
        if path and (path:match("/rextd$") or
            (path == "/usr/bin/login" and block.label == "rextd" and not process.last_exit)) then
          return id
        end
      end
      return nil
    end
  end
end

rex.action({
  name = "rextd_show_host",
  title = text("rextd / Show Host 1", "rextd / 显示 1 号主机"),
  description = text("Focus rextd in this Rex window and filter Attention by host number", "聚焦当前窗口的 rextd，按主机编号筛选需要处理列表"),
  category = "rextd",
  keywords = "tasks sidebar host filter 任务 侧栏 主机 筛选",
  args = { slot = "integer?" },
  run = function(ctx, args)
    local slot = args.slot or 1
    if slot < 0 or slot > 9 or slot % 1 ~= 0 then
      error(text("Host number must be between 0 and 9.", "主机编号必须在 0 到 9 之间。"), 0)
    end
    local view, view_error = rex.call("session.view", { session_id = ctx.session_id })
    if not view then error(view_error, 0) end
    local target = sidebar_in_window(ctx, view)
    if not target then
      error(text("Open rextd in this Rex window first.", "请先在当前 Rex 窗口打开 rextd。"), 0)
    end
    local result, write_error = rex.call(terminal .. ".write", {
      session_id = ctx.session_id,
      block_id = target,
      args = { data = string.char(27) .. tostring(slot) },
    })
    if not result then error(write_error, 0) end
    local focused, focus_error = rex.call("session.focus_block", { session_id = ctx.session_id, block_id = target })
    if not focused then error(focus_error, 0) end
    return { block_id = target }
  end,
})
