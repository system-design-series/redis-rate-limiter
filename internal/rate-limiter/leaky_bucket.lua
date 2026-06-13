-- Atomic leaky-bucket leak + admit.
-- Models a bucket that leaks at a constant rate; each request adds `cost`
-- units. The request is allowed only if it fits under `capacity`, so the
-- long-run throughput can never exceed the leak rate while short bursts are
-- absorbed by the bucket's spare room.
--
-- KEYS[1] = bucket key (hash of { level, ts_ms })
-- ARGV[1] = capacity (max units the bucket holds)
-- ARGV[2] = leak_per_sec (units drained per second)
-- ARGV[3] = cost (units this request adds)
-- Returns: { allowed(0|1), remaining, retry_after_ms, reset_after_ms }

local key      = KEYS[1]
local capacity = tonumber(ARGV[1])
local leak     = tonumber(ARGV[2])
local cost     = tonumber(ARGV[3])

-- Single authoritative clock: the Redis server's TIME, in milliseconds.
local t = redis.call('TIME')
local now_ms = (tonumber(t[1]) * 1000) + math.floor(tonumber(t[2]) / 1000)

local data  = redis.call('HMGET', key, 'level', 'ts_ms')
local level = tonumber(data[1])
local ts_ms = tonumber(data[2])

-- Cold bucket starts empty.
if level == nil or ts_ms == nil then
  level = 0
  ts_ms = now_ms
end

-- Leak by elapsed time; clamp negative elapsed (clock jumps) to zero.
local elapsed = now_ms - ts_ms
if elapsed < 0 then elapsed = 0 end
level = math.max(0, level - (elapsed / 1000.0) * leak)

-- Admit only if the request fits without overflowing the bucket.
local allowed = 0
if level + cost <= capacity then
  level = level + cost
  allowed = 1
end

-- Persist. Store level as a string to preserve the fraction exactly; pin the
-- numeric formats (don't depend on Lua's default %.14g): ts_ms is an exact
-- integer.
redis.call('HSET', key, 'level', string.format('%.6f', level), 'ts_ms', string.format('%d', now_ms))

-- TTL = time to drain fully, floored at 1s to avoid sub-tick eviction churn.
local reset_ms = math.ceil((level / leak) * 1000)
local ttl_ms   = reset_ms
if ttl_ms < 1000 then ttl_ms = 1000 end
redis.call('PEXPIRE', key, ttl_ms)

-- When rejected, time until enough has drained for `cost` to fit.
local retry_ms = 0
if allowed == 0 then
  local overflow = (level + cost) - capacity
  retry_ms = math.ceil((overflow / leak) * 1000)
end

-- remaining = whole units of free room left in the bucket.
local remaining = math.floor(capacity - level)

return { allowed, remaining, retry_ms, reset_ms }
