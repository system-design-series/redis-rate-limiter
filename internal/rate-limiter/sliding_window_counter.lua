-- Atomic sliding-window-counter estimate + admit.
-- Approximates a rolling window by blending the current fixed window with the
-- previous one, weighted by how much of the previous window still overlaps the
-- rolling window ending "now". This removes most of the fixed-window boundary
-- burst while staying O(1) in time and memory.
--
-- KEYS[1] = counter key (hash of { window, cur, prev })
-- ARGV[1] = limit (max requests per window)
-- ARGV[2] = window_sec (window length in seconds)
-- ARGV[3] = cost (requests this call counts)
-- Returns: { allowed(0|1), remaining, retry_after_ms, reset_after_ms }

local key       = KEYS[1]
local limit     = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2]) * 1000
local cost      = tonumber(ARGV[3])

-- Single authoritative clock: the Redis server's TIME, in milliseconds.
local t = redis.call('TIME')
local now_ms = (tonumber(t[1]) * 1000) + math.floor(tonumber(t[2]) / 1000)

-- Which fixed window are we in, and how far into it?
local cur_window = math.floor(now_ms / window_ms)
local elapsed    = now_ms - (cur_window * window_ms)

local data       = redis.call('HMGET', key, 'window', 'cur', 'prev')
local stored_win = tonumber(data[1])
local cur        = tonumber(data[2])
local prev       = tonumber(data[3])

if stored_win == nil then
  -- Cold start.
  cur, prev = 0, 0
elseif stored_win == cur_window then
  -- Same window: keep both counts.
  cur  = cur or 0
  prev = prev or 0
elseif stored_win == cur_window - 1 then
  -- Advanced exactly one window: last window's count becomes "previous".
  prev = cur or 0
  cur  = 0
else
  -- Gap of two or more windows: both old counts are stale.
  cur, prev = 0, 0
end

-- Weight = fraction of the previous window still inside the rolling view.
-- Shrinks from 1 -> 0 as we move through the current window.
local weight   = (window_ms - elapsed) / window_ms
local estimate = cur + (prev * weight)

local allowed = 0
if estimate + cost <= limit then
  cur     = cur + cost
  allowed = 1
end

-- Persist and keep for two windows so the "previous" count survives the roll.
redis.call('HSET', key, 'window', cur_window, 'cur', cur, 'prev', prev)
redis.call('PEXPIRE', key, window_ms * 2)

-- Recompute the estimate after this decision for the response headers.
local est_after = cur + (prev * weight)
local remaining = math.floor(limit - est_after)
if remaining < 0 then remaining = 0 end

-- reset = time until the current fixed window rolls over.
local reset_ms = ((cur_window + 1) * window_ms) - now_ms

-- When rejected, approximate retry as the time until the window rolls over, at
-- which point the previous window's contribution fully expires. This is a
-- simple estimate, not an exact solve.
local retry_ms = 0
if allowed == 0 then
  retry_ms = reset_ms
end

return { allowed, remaining, retry_ms, reset_ms }
