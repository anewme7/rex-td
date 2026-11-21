-- Load on a monitored Rex server. Linux uses the kernel's aggregate counters.
-- For macOS, set rex_td_preview_binary to the build on this server before dofile.
rex.action({
  name = "rex_td_cpu_snapshot",
  title = "rextd CPU counters",
  run = function()
    local proc = io.open("/proc/stat", "r")
    if proc then
      local line = proc:read("*l") or ""
      proc:close()
      local ticks = {}
      for value in line:gmatch("%d+") do
        if #ticks == 8 then break end
        ticks[#ticks + 1] = tonumber(value)
      end
      if line:match("^cpu%s") and #ticks == 8 then
        return {
          Busy = ticks[1] + ticks[2] + ticks[3] + ticks[6] + ticks[7] + ticks[8],
          Idle = ticks[4] + ticks[5],
        }
      end
      error("Could not read aggregate /proc/stat CPU counters", 0)
    end
    if not rex_td_preview_binary then
      error("Set rex_td_preview_binary to the local rextd executable", 0)
    end
    local result, err = rex.exec({ rex_td_preview_binary, "--cpu-snapshot" }, { timeout = 2 })
    if not result or result.code ~= 0 then error(err or "Could not read native CPU counters", 0) end
    local counters, decode_error = rex.json.decode(result.stdout)
    if not counters then error(decode_error, 0) end
    return counters
  end,
})
