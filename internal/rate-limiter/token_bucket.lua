-- Atomic token-bucket refill + decrement.
-- KEYS[1] = bucket key (hash of { tokens, ts_ms })
-- ARGV[1] = capacity (max tokens)
-- ARGV[2] = refill_per_sec (tokens added per second)
-- ARGV[3] = cost (tokens this request consumes)
-- Returns: { allowed(0|1), remaining, retry_after_ms, reset_after_ms }

local key      = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill   = tonumber(ARGV[2])
local cost     = tonumber(ARGV[3])

-- Single authoritative clock: the Redis server's TIME, in milliseconds.
local t = redis.call('TIME')
local now_ms = (tonumber(t[1]) * 1000) + math.floor(tonumber(t[2]) / 1000)

local data   = redis.call('HMGET', key, 'tokens', 'ts_ms')
local tokens = tonumber(data[1])
local ts_ms  = tonumber(data[2])

-- Cold bucket starts full (burst allowance).
if tokens == nil or ts_ms == nil then
  tokens = capacity
  ts_ms  = now_ms
end

-- Refill by elapsed time; clamp negative elapsed (clock jumps) to zero.
local elapsed = now_ms - ts_ms
if elapsed < 0 then elapsed = 0 end
tokens = math.min(capacity, tokens + (elapsed / 1000.0) * refill)

local allowed = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
end

-- Persist. Store as strings to preserve the fractional token count exactly.
-- Pin numeric formats (don't depend on Lua's default %.14g): tokens keeps its
-- fraction, ts_ms is an exact integer.
redis.call('HSET', key, 'tokens', string.format('%.6f', tokens), 'ts_ms', string.format('%d', now_ms))

-- TTL = time to refill to full, floored at 1s to avoid sub-tick eviction churn.
local missing  = capacity - tokens
local reset_ms = math.ceil((missing / refill) * 1000)
local ttl_ms   = reset_ms
if ttl_ms < 1000 then ttl_ms = 1000 end
redis.call('PEXPIRE', key, ttl_ms)

local retry_ms = 0
if allowed == 0 then
  retry_ms = math.ceil(((cost - tokens) / refill) * 1000)
end

return { allowed, math.floor(tokens), retry_ms, reset_ms }
