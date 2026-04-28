-- jerboa.lua — Jerboa <-> Reaper sync panel
--
-- Drop-in: drop this single file into Reaper's Scripts/ folder, run it,
-- click "Connect to Jerboa" once. The script handles everything else
-- (browser auth, downloading the platform-specific sync binary into the
-- script's directory). No ReaPack extensions needed — uses Reaper's
-- built-in gfx API and curl (ships with Win10+, macOS, Linux).
--
-- Buttons (after connect):
--   Refresh    — re-read manifest from server
--   Bake stale — solo+render each non-clean track to .jerboa-bake/<guid>.wav,
--                then upload via the sync binary
--   Sync       — full push (raw media + .rpp) + upload baked + pull
--
-- Long-running work runs as a coroutine that yields between heavy steps,
-- so the panel keeps painting frames during multi-track bakes.

local r = reaper

-- Default Jerboa server. Override here if you self-host.
local JERBOA_SERVER = "https://jerboa.app"

------------------------------------------------------------------- platform

local IS_WIN = package.config:sub(1, 1) == "\\"

local function detect_platform()
    local os_str = r.GetOS() or ""
    if os_str:find("Win") then return "windows" end
    -- macOS support is a future add — current binary builds are linux/windows.
    return "linux"
end

local function script_dir()
    local _, fn = r.get_action_context()
    return (fn or ""):match("(.*[/\\])") or "./"
end

local function bin_path()
    local name = "jerboa-sync"
    if IS_WIN then name = name .. ".exe" end
    return script_dir() .. name
end

local function bin_exists()
    local f = io.open(bin_path(), "rb")
    if f then f:close(); return true end
    return false
end

-- Quoted absolute path to the bundled binary, suitable for run_capture.
local function jerboa_sync_cmd()
    if IS_WIN then return '"' .. bin_path() .. '"' end
    return "'" .. bin_path():gsub("'", "'\\''") .. "'"
end

------------------------------------------------------------------- state

local state = {
    meta = {}, tracks = {}, log = {},
    busy = nil, coro = nil,
    auto_refreshed = false,
    prev_mouse = 0,
    click = false,
}

local function log(s)
    state.log[#state.log + 1] = os.date("%H:%M:%S  ") .. tostring(s)
    if #state.log > 300 then table.remove(state.log, 1) end
end

------------------------------------------------------------------- shell

local function shell_quote(s)
    if package.config:sub(1, 1) == "\\" then
        return '"' .. s:gsub('"', '\\"') .. '"'
    end
    return "'" .. s:gsub("'", "'\\''") .. "'"
end

local function run_capture(cmd, cwd)
    local full = cmd
    if cwd then
        if package.config:sub(1, 1) == "\\" then
            full = 'cd /d ' .. shell_quote(cwd) .. ' && ' .. cmd
        else
            full = 'cd ' .. shell_quote(cwd) .. ' && ' .. cmd
        end
    end
    local h = io.popen(full .. " 2>&1")
    if not h then return nil, false end
    local out = h:read("*a")
    local ok = h:close()
    return out, ok
end

------------------------------------------------------------------- project

local function project_rpp_path()
    local _, fn = r.EnumProjects(-1, "")
    if not fn or fn == "" then return nil end
    return fn
end

local function dirname(p) return p:match("(.*[/\\])") or "./" end
local function strip_braces(s) return (s:gsub("^{(.-)}$", "%1")) end

local function find_track_by_guid(guid)
    guid = strip_braces(guid):lower()
    for i = 0, r.CountTracks(0) - 1 do
        local t = r.GetTrack(0, i)
        local _, g = r.GetSetMediaTrackInfo_String(t, "GUID", "", false)
        if strip_braces(g):lower() == guid then return t end
    end
    return nil
end

local function parse_manifest(text)
    local meta, tracks = {}, {}
    for line in text:gmatch("[^\r\n]+") do
        local k, v = line:match("^# (%w+)=(.*)$")
        if k then
            meta[k] = v
        elseif line:sub(1, 1) ~= "#" and line ~= "" then
            local g, n, h, st = line:match("^([^\t]+)\t([^\t]*)\t([^\t]+)\t([^\t]+)$")
            if g then tracks[#tracks + 1] = { guid = g, name = n, hash = h, status = st } end
        end
    end
    return meta, tracks
end

------------------------------------------------------------------- render

local function snapshot_solo()
    local s = {}
    for i = 0, r.CountTracks(0) - 1 do
        s[i] = r.GetMediaTrackInfo_Value(r.GetTrack(0, i), "I_SOLO")
    end
    return s
end
local function restore_solo(s)
    for i = 0, r.CountTracks(0) - 1 do
        r.SetMediaTrackInfo_Value(r.GetTrack(0, i), "I_SOLO", s[i] or 0)
    end
end
local function solo_only(track)
    for i = 0, r.CountTracks(0) - 1 do
        r.SetMediaTrackInfo_Value(r.GetTrack(0, i), "I_SOLO", 0)
    end
    r.SetMediaTrackInfo_Value(track, "I_SOLO", 2)
end

local function bake_one(track, guid, bake_dir)
    solo_only(track)
    r.GetSetProjectInfo_String(0, "RENDER_FILE", bake_dir, true)
    r.GetSetProjectInfo_String(0, "RENDER_PATTERN", guid, true)
    r.GetSetProjectInfo(0, "RENDER_SETTINGS", 0, true)   -- master mix
    r.GetSetProjectInfo(0, "RENDER_BOUNDSFLAG", 1, true) -- entire project
    r.GetSetProjectInfo(0, "RENDER_CHANNELS", 2, true)
    r.GetSetProjectInfo(0, "RENDER_SRATE", 0, true)
    r.GetSetProjectInfo(0, "RENDER_TAILFLAG", 0xFF, true)
    r.Main_OnCommand(42230, 0)
end

------------------------------------------------------------------- tasks

local function step(msg) state.busy = msg; coroutine.yield() end

local function refresh_now()
    local rpp = project_rpp_path()
    if not rpp then return false, "project must be saved" end
    local out, ok = run_capture(jerboa_sync_cmd() .. " --manifest", dirname(rpp))
    if not ok then return false, "manifest failed: " .. (out or "") end
    state.meta, state.tracks = parse_manifest(out or "")
    return true
end

local function task_refresh()
    step("Saving project...")
    r.Main_SaveProject(0, false)
    step("Fetching manifest...")
    local ok, err = refresh_now()
    if not ok then log("error: " .. err); return end
    log(string.format("refresh — %d track(s)", #state.tracks))
end

local function task_bake()
    step("Saving project...")
    r.Main_SaveProject(0, false)
    step("Fetching manifest...")
    local ok, err = refresh_now()
    if not ok then log("error: " .. err); return end
    if not state.meta.bake_dir or state.meta.bake_dir == "" then
        log("error: missing bake_dir"); return
    end
    r.RecursiveCreateDirectory(state.meta.bake_dir, 0)

    local snap = snapshot_solo()
    r.PreventUIRefresh(1)
    local n = 0
    for _, t in ipairs(state.tracks) do
        if t.status ~= "clean" then
            local mt = find_track_by_guid(t.guid)
            local label = (t.name ~= "" and t.name) or t.guid:sub(1, 8)
            if mt then
                step("Baking " .. label .. " ...")
                bake_one(mt, t.guid, state.meta.bake_dir)
                log("baked " .. label)
                n = n + 1
            else
                log("skip " .. label .. " (track GUID not in project)")
            end
        end
    end
    restore_solo(snap)
    r.PreventUIRefresh(-1)
    r.UpdateArrange()
    log(string.format("bake — %d track(s)", n))

    step("Uploading...")
    local pout, pok = run_capture(jerboa_sync_cmd(), dirname(project_rpp_path()))
    if pout then for line in pout:gmatch("[^\r\n]+") do log(line) end end
    if not pok then log("upload exited non-zero") end

    step("Refreshing...")
    refresh_now()
end

------------------------------------------------------------------- connect

-- minimal JSON-string field extractor; sufficient for our small responses
local function jstr(s, key)
    return s:match('"' .. key .. '"%s*:%s*"([^"]*)"')
end

local function jescape(s)
    return s:gsub('"', '\\"')
end

local function http_post_json(url, body)
    local cmd = string.format(
        'curl -fsSL -X POST -H "Content-Type: application/json" -d %s %s',
        shell_quote(body), shell_quote(url)
    )
    return run_capture(cmd)
end

local function open_browser(url)
    local cmd
    if IS_WIN then
        cmd = 'start "" ' .. shell_quote(url)
    elseif r.GetOS():find("OSX") or r.GetOS():find("macOS") then
        cmd = 'open ' .. shell_quote(url)
    else
        cmd = 'xdg-open ' .. shell_quote(url)
    end
    os.execute(cmd .. " >/dev/null 2>&1 &")
end

-- Sleeps for `secs` seconds while keeping the UI responsive: yields once,
-- then busy-waits using os.clock(). Coarse but works without timers.
local function coro_sleep(secs)
    local t0 = os.clock()
    while os.clock() - t0 < secs do
        coroutine.yield()
    end
end

local function task_connect()
    step("Requesting pairing code...")
    local body = '{}'
    local out, ok = http_post_json(JERBOA_SERVER .. "/api/pair/start", body)
    if not ok or not out then
        log("connect: start failed: " .. (out or "no response"))
        return
    end
    local code = jstr(out, "code")
    local verify_url = jstr(out, "verify_url")
    if not code or not verify_url then
        log("connect: bad response: " .. out)
        return
    end
    log("pairing code: " .. code)
    log("opening browser: " .. verify_url)
    open_browser(verify_url)

    step("Waiting for browser authorization (code: " .. code .. ")...")
    local poll_body = string.format('{"code":"%s"}', jescape(code))
    local token, server_url, band_slug
    local deadline = os.time() + 600 -- 10 minutes
    while os.time() < deadline do
        coro_sleep(2)
        local pout, pok = http_post_json(JERBOA_SERVER .. "/api/pair/poll", poll_body)
        if not pok then
            -- 404 from curl -f is non-zero exit; treat as fatal (code expired)
            log("connect: code expired or invalid")
            return
        end
        local status = jstr(pout or "", "status")
        if status == "authorized" then
            token = jstr(pout, "token")
            server_url = jstr(pout, "server_url")
            band_slug = jstr(pout, "band_slug")
            break
        end
    end
    if not token then
        log("connect: timed out waiting for authorization")
        return
    end
    log("authorized for band: " .. (band_slug or "?"))

    step("Downloading sync binary...")
    local platform = detect_platform()
    local dl_url = string.format("%s/api/bands/%s/sync/binary?platform=%s",
        server_url or JERBOA_SERVER, band_slug, platform)
    local dl_cmd = string.format(
        'curl -fsSL -H "Authorization: Bearer %s" %s -o %s',
        token, shell_quote(dl_url), shell_quote(bin_path())
    )
    local dout, dok = run_capture(dl_cmd)
    if not dok then
        log("connect: binary download failed: " .. (dout or ""))
        return
    end
    if not IS_WIN then
        os.execute('chmod +x ' .. shell_quote(bin_path()))
    end
    if not bin_exists() then
        log("connect: binary not found after download")
        return
    end
    log("connected — sync binary installed at " .. bin_path())
end

local function task_sync()
    step("Saving project...")
    r.Main_SaveProject(0, false)
    step("Syncing...")
    local rpp = project_rpp_path()
    if not rpp then log("error: project must be saved"); return end
    local out, ok = run_capture(jerboa_sync_cmd(), dirname(rpp))
    if out then for line in out:gmatch("[^\r\n]+") do log(line) end end
    if not ok then log("sync exited non-zero") end
    step("Refreshing...")
    refresh_now()
end

local function start(fn, label)
    if state.coro then return end
    log(label .. "...")
    state.coro = coroutine.create(fn)
    state.busy = label
end

------------------------------------------------------------------- gfx

gfx.init("Jerboa Sync", 720, 540, 0, 200, 200)
gfx.setfont(1, "Sans", 14)

local function setcol(rr, gg, bb, aa)
    gfx.set(rr / 255, gg / 255, bb / 255, aa or 1)
end

local function point_in(x, y, w, h, mx, my)
    return mx >= x and mx < x + w and my >= y and my < y + h
end

local function lmb_down() return gfx.mouse_cap % 2 >= 1 end

-- button: returns true on the frame the LMB transitions down inside it
local function button(x, y, w, h, label, disabled)
    local hot = point_in(x, y, w, h, gfx.mouse_x, gfx.mouse_y)
    if disabled then
        setcol(50, 50, 55)
    elseif hot and lmb_down() then
        setcol(95, 95, 120)
    elseif hot then
        setcol(75, 75, 95)
    else
        setcol(55, 55, 70)
    end
    gfx.rect(x, y, w, h, 1)
    setcol(disabled and 80 or 200, disabled and 80 or 200, disabled and 90 or 215)
    gfx.rect(x, y, w, h, 0)
    local tw, th = gfx.measurestr(label)
    gfx.x = x + math.floor((w - tw) / 2)
    gfx.y = y + math.floor((h - th) / 2)
    setcol(disabled and 110 or 230, disabled and 110 or 230, disabled and 115 or 235)
    gfx.drawstr(label)
    return (not disabled) and hot and state.click
end

local STATUS_COL = {
    clean   = { 74, 222, 128 },
    stale   = { 251, 191, 36 },
    unbaked = { 244, 114, 182 },
}

local function frame()
    -- click edge: LMB low->high this frame
    local cap = gfx.mouse_cap
    state.click = (cap % 2 >= 1) and (state.prev_mouse % 2 == 0)
    state.prev_mouse = cap

    setcol(28, 28, 32)
    gfx.rect(0, 0, gfx.w, gfx.h, 1)

    local pad = 12
    local y = pad

    local connected = bin_exists()

    -- header
    gfx.x = pad; gfx.y = y
    if not connected then
        setcol(140, 140, 150)
        gfx.drawstr("Not connected — click Connect to authorize a band.")
    elseif state.meta.session_name and state.meta.session_name ~= "" then
        setcol(220, 220, 230)
        gfx.drawstr("Session: " .. state.meta.session_name)
    else
        setcol(140, 140, 150)
        gfx.drawstr("No session loaded — open a saved .rpp and click Refresh.")
    end
    y = y + 24

    -- buttons
    local disabled = state.coro ~= nil
    local bw, bh = 110, 28
    if not connected then
        if button(pad, y, 180, bh, "Connect to Jerboa", disabled) then
            start(task_connect, "Connect")
        end
        if state.busy then
            setcol(150, 150, 160)
            gfx.x = pad + 190
            gfx.y = y + math.floor((bh - 14) / 2)
            gfx.drawstr(state.busy)
        end
    else
        if button(pad,                y, bw, bh, "Refresh",    disabled) then start(task_refresh, "Refresh") end
        if button(pad + (bw + 8),     y, bw, bh, "Bake stale", disabled) then start(task_bake,    "Bake stale") end
        if button(pad + (bw + 8) * 2, y, bw, bh, "Sync",       disabled) then start(task_sync,    "Sync") end
        if state.busy then
            setcol(150, 150, 160)
            gfx.x = pad + (bw + 8) * 3 + 10
            gfx.y = y + math.floor((bh - 14) / 2)
            gfx.drawstr(state.busy)
        end
    end
    y = y + bh + 10

    setcol(60, 60, 70)
    gfx.line(pad, y, gfx.w - pad, y)
    y = y + 6

    -- tracks table
    local row_h = 20
    setcol(160, 160, 170)
    gfx.x = pad;          gfx.y = y; gfx.drawstr("Track")
    gfx.x = gfx.w - 230;  gfx.y = y; gfx.drawstr("Status")
    gfx.x = gfx.w - 110;  gfx.y = y; gfx.drawstr("Hash")
    y = y + row_h

    local table_h = math.floor((gfx.h - y - pad) * 0.55)
    local table_top = y
    local table_bottom = y + table_h - row_h
    for i, t in ipairs(state.tracks) do
        if y > table_bottom then break end
        if i % 2 == 0 then
            setcol(40, 40, 48)
            gfx.rect(pad - 4, y - 2, gfx.w - 2 * pad + 8, row_h, 1)
        end
        setcol(220, 220, 225)
        gfx.x = pad; gfx.y = y
        gfx.drawstr((t.name ~= "" and t.name) or "(unnamed)")
        local sc = STATUS_COL[t.status] or { 200, 200, 200 }
        setcol(sc[1], sc[2], sc[3])
        gfx.x = gfx.w - 230; gfx.y = y; gfx.drawstr(t.status)
        setcol(140, 140, 150)
        gfx.x = gfx.w - 110; gfx.y = y; gfx.drawstr(t.hash:sub(1, 8))
        y = y + row_h
    end
    y = table_top + table_h

    setcol(60, 60, 70)
    gfx.line(pad, y, gfx.w - pad, y)
    y = y + 6

    -- log pane (last N lines that fit)
    setcol(160, 160, 170)
    gfx.x = pad; gfx.y = y; gfx.drawstr("Log")
    y = y + 18
    local log_h = gfx.h - y - pad
    local lines_visible = math.max(1, math.floor(log_h / 16))
    local start_idx = math.max(1, #state.log - lines_visible + 1)
    setcol(190, 190, 200)
    for i = start_idx, #state.log do
        gfx.x = pad; gfx.y = y; gfx.drawstr(state.log[i])
        y = y + 16
    end
end

------------------------------------------------------------------- loop

local function loop()
    frame()

    if state.coro then
        local ok, err = coroutine.resume(state.coro)
        if not ok then log("error: " .. tostring(err)) end
        if coroutine.status(state.coro) == "dead" then
            state.coro = nil
            state.busy = nil
        end
    end

    if not state.auto_refreshed and not state.coro and bin_exists() then
        state.auto_refreshed = true
        if project_rpp_path() then start(task_refresh, "Refresh") end
    end

    gfx.update()
    local c = gfx.getchar()
    if c >= 0 then
        r.defer(loop)
    else
        gfx.quit()
    end
end

loop()
