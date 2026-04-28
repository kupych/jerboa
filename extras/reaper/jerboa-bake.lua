-- jerboa-bake.lua
--
-- Bakes stale Reaper tracks (post-FX, automation, fader, sends) into
-- per-track WAVs and uploads them to Jerboa via the jerboa-sync binary.
--
-- Workflow:
--   1. shell out to `jerboa-sync --manifest` in the project dir
--      → reads stale-track list (GUID, name, render_hash) on stdout
--   2. for each stale track: solo it, render master mix to
--      .jerboa-bake/<guid>.wav, restore solo state
--   3. shell out to `jerboa-sync` (normal mode) → uploads the freshly
--      baked WAVs and pulls anything new from the server.
--
-- Requirements:
--   * jerboa-sync binary on PATH (or set JERBOA_SYNC_BIN below)
--   * project saved (the .rpp path is what we bake against)
--   * render format set to WAV in Reaper's render dialog (one-time)

local r = reaper

local JERBOA_SYNC_BIN = "jerboa-sync" -- override with absolute path if needed

------------------------------------------------------------------- helpers

local function msg(s)
    r.ShowConsoleMsg(tostring(s) .. "\n")
end

local function shell_quote(s)
    if package.config:sub(1, 1) == "\\" then
        -- Windows: wrap in quotes, escape internal quotes
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
    if not h then return nil, "popen failed" end
    local out = h:read("*a")
    local ok, _, code = h:close()
    return out, ok, code
end

local function project_rpp_path()
    local _, fn = r.EnumProjects(-1, "")
    if not fn or fn == "" then return nil end
    return fn
end

local function dirname(path)
    return path:match("(.*[/\\])") or "./"
end

local function strip_braces(s)
    return (s:gsub("^{(.-)}$", "%1"))
end

local function find_track_by_guid(guid)
    guid = strip_braces(guid):lower()
    for i = 0, r.CountTracks(0) - 1 do
        local t = r.GetTrack(0, i)
        local _, g = r.GetSetMediaTrackInfo_String(t, "GUID", "", false)
        if strip_braces(g):lower() == guid then
            return t
        end
    end
    return nil
end

local function ensure_dir(path)
    r.RecursiveCreateDirectory(path, 0)
end

------------------------------------------------------------------- manifest

local function parse_manifest(text)
    local meta = {}
    local stale = {}
    for line in text:gmatch("[^\r\n]+") do
        local k, v = line:match("^# (%w+)=(.*)$")
        if k then
            meta[k] = v
        elseif line:sub(1, 1) ~= "#" and line ~= "" then
            local guid, name, hash, status = line:match("^([^\t]+)\t([^\t]*)\t([^\t]+)\t([^\t]+)$")
            if guid and hash and status and status ~= "clean" then
                stale[#stale + 1] = { guid = guid, name = name, hash = hash, status = status }
            end
        end
    end
    return meta, stale
end

------------------------------------------------------------------- render

-- snapshot solo state so we can restore it after baking
local function snapshot_solo()
    local snap = {}
    for i = 0, r.CountTracks(0) - 1 do
        local t = r.GetTrack(0, i)
        snap[i] = r.GetMediaTrackInfo_Value(t, "I_SOLO")
    end
    return snap
end

local function restore_solo(snap)
    for i = 0, r.CountTracks(0) - 1 do
        local t = r.GetTrack(0, i)
        r.SetMediaTrackInfo_Value(t, "I_SOLO", snap[i] or 0)
    end
end

local function solo_only(track)
    for i = 0, r.CountTracks(0) - 1 do
        local t = r.GetTrack(0, i)
        r.SetMediaTrackInfo_Value(t, "I_SOLO", 0)
    end
    -- 2 = solo (in place); ignores routing so sends still feed master
    r.SetMediaTrackInfo_Value(track, "I_SOLO", 2)
end

-- Configure render settings and invoke render-without-dialog.
-- Master-mix mode means each track is rendered with its full signal path
-- to master (FX, automation, fader, sends), which is exactly the preview
-- we want.
local function bake_one(track, guid, bake_dir)
    solo_only(track)

    r.GetSetProjectInfo_String(0, "RENDER_FILE", bake_dir, true)
    r.GetSetProjectInfo_String(0, "RENDER_PATTERN", guid, true)
    r.GetSetProjectInfo(0, "RENDER_SETTINGS", 0, true)   -- master mix
    r.GetSetProjectInfo(0, "RENDER_BOUNDSFLAG", 1, true) -- entire project
    r.GetSetProjectInfo(0, "RENDER_CHANNELS", 2, true)
    r.GetSetProjectInfo(0, "RENDER_SRATE", 0, true)      -- project sample rate
    r.GetSetProjectInfo(0, "RENDER_TAIL", 1, true)
    r.GetSetProjectInfo(0, "RENDER_TAILFLAG", 0xFF, true)

    -- 42230 = File: Render project, using the most recent render settings
    r.Main_OnCommand(42230, 0)
end

------------------------------------------------------------------- main

local function main()
    r.ClearConsole()
    msg("jerboa-bake — starting")

    local rpp = project_rpp_path()
    if not rpp or rpp == "" then
        msg("error: project must be saved as a .rpp file before baking")
        return
    end
    local proj_dir = dirname(rpp)
    msg("project: " .. rpp)

    -- Save first so jerboa-sync sees the latest .rpp on disk.
    r.Main_SaveProject(0, false)

    msg("\n[1/3] fetching manifest ...")
    local out, ok, code = run_capture(JERBOA_SYNC_BIN .. " --manifest", proj_dir)
    if not ok then
        msg("error: jerboa-sync --manifest failed (exit " .. tostring(code) .. ")")
        msg(out or "")
        return
    end
    local meta, stale = parse_manifest(out)

    if not meta.bake_dir or meta.bake_dir == "" then
        msg("error: manifest missing bake_dir")
        msg(out)
        return
    end
    ensure_dir(meta.bake_dir)

    if #stale == 0 then
        msg("nothing to bake — all tracks up-to-date")
    else
        msg(string.format("\n[2/3] baking %d track(s) → %s", #stale, meta.bake_dir))
        local snap = snapshot_solo()
        r.PreventUIRefresh(1)
        for i, s in ipairs(stale) do
            local t = find_track_by_guid(s.guid)
            local label = (s.name ~= "" and s.name) or s.guid:sub(1, 8)
            if not t then
                msg(string.format("  [%d/%d] skip  %s (track GUID not found in project)", i, #stale, label))
            else
                msg(string.format("  [%d/%d] bake  %s", i, #stale, label))
                bake_one(t, s.guid, meta.bake_dir)
            end
        end
        restore_solo(snap)
        r.PreventUIRefresh(-1)
        r.UpdateArrange()
    end

    msg("\n[3/3] uploading via jerboa-sync ...")
    local push_out, push_ok, push_code = run_capture(JERBOA_SYNC_BIN, proj_dir)
    msg(push_out or "")
    if not push_ok then
        msg("jerboa-sync exited " .. tostring(push_code))
    end

    msg("\njerboa-bake — done")
end

main()
