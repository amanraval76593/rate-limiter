local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local unique_id = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

local count = redis.call('ZCARD', key)

if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    if oldest and oldest[2] then
        local oldest_score = tonumber(oldest[2])
        return {0, math.max(0, oldest_score + window - now)}
    end
    return {0, window}
else
    redis.call('ZADD', key, now, now .. ':' .. unique_id)
    redis.call('EXPIRE', key, window)
    return {1, 0}
end
