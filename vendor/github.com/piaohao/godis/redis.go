package godis

import (
	"sync"
	"time"
)

// Option connect options
type Option struct {
	Host              string        // redis host
	Port              int           // redis port
	ConnectionTimeout time.Duration // connect timeout
	SoTimeout         time.Duration // read timeout
	Password          string        // redis password,if empty,then without auth
	Db                int           // which db to connect
}

// Redis redis client tool
type Redis struct {
	client      *client
	pipeline    *Pipeline
	transaction *Transaction
	dataSource  *Pool
	activeTime  time.Time

	mu sync.RWMutex
}

//NewRedis constructor for creating new redis
func NewRedis(option *Option) *Redis {
	client := newClient(option)
	return &Redis{client: client}
}

//Connect connect to redis
func (r *Redis) Connect() error {
	return r.client.connect()
}

//Close close redis connection
func (r *Redis) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dataSource != nil {
		if r.client.broken {
			return r.dataSource.returnBrokenResourceObject(r)
		}
		return r.dataSource.returnResourceObject(r)

	}
	if r.client != nil {
		return r.client.close()
	}
	return nil
}

func (r *Redis) setDataSource(pool *Pool) {
	r.mu.Lock()
	r.dataSource = pool
	r.mu.Unlock()
}

// Send send command to redis
func (r *Redis) Send(command protocolCommand, args ...[]byte) error {
	return r.client.sendCommand(command, args...)
}

// SendByStr send command to redis
func (r *Redis) SendByStr(command string, args ...[]byte) error {
	return r.client.sendCommandByStr(command, args...)
}

// Receive receive reply from redis
func (r *Redis) Receive() (interface{}, error) {
	return r.client.getOne()
}

// check current redis is in transaction or pipeline mode
// if yes,then cannot execute command in redis mode
func (r *Redis) checkIsInMultiOrPipeline() error {
	if r.client.isInMulti {
		return newDataError("cannot use Redis when in Multi. Please use Transaction or reset redis state")
	}
	if r.pipeline != nil && len(r.pipeline.pipelinedResponses) > 0 {
		return newDataError("cannot use Redis when in Pipeline. Please use Pipeline or reset redis state")
	}
	return nil
}

//<editor-fold desc="rediscommands">

// Set the string value as value of the key. The string can't be longer than 1073741824 bytes (1 GB)
// return Status code reply
func (r *Redis) Set(key, value string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.set(key, value)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//SetWithParamsAndTime Set the string value as value of the key. The string can't be longer than 1073741824 bytes (1 GB).
// param nxxx NX|XX, NX -- Only set the key if it does not already exist. XX -- Only set the key if it already exist.
// param expx EX|PX, expire time units: EX = seconds; PX = milliseconds
// param time expire time in the units of <code>expx</code>
//return Status code reply
func (r *Redis) SetWithParamsAndTime(key, value, nxxx, expx string, time int64) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.setWithParamsAndTime(key, value, nxxx, expx, time)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Get the value of the specified key. If the key does not exist null is returned. If the value
//stored at key is not a string an error is returned because GET can only handle string values.
//param key
//return Bulk reply
func (r *Redis) Get(key string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.get(key)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//Type Return the type of the value stored at key in form of a string. The type can be one of "none",
//"string", "list", "set". "none" is returned if the key does not exist. Time complexity: O(1)
//param key
//return Status code reply, specifically: "none" if the key does not exist "string" if the key
//        contains a String value "list" if the key contains a List value "set" if the key
//        contains a Set value "zset" if the key contains a Sorted Set value "hash" if the key
//        contains a Hash value
func (r *Redis) Type(key string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.typeKey(key)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//Expire Set a timeout on the specified key. After the timeout the key will be automatically deleted by
//the server. A key with an associated timeout is said to be volatile in Redis terminology.
//
//Voltile keys are stored on disk like the other keys, the timeout is persistent too like all the
//other aspects of the dataset. Saving a dataset containing expires and stopping the server does
//not stop the flow of time as Redis stores on disk the time when the key will no longer be
//available as Unix time, and not the remaining seconds.
//
//Since Redis 2.1.3 you can update the value of the timeout of a key already having an expire
//set. It is also possible to undo the expire at all turning the key into a normal key using the
//{@link #persist(String) PERSIST} command.
//
//return Integer reply, specifically: 1: the timeout was set. 0: the timeout was not set since
//        the key already has an associated timeout (this may happen only in Redis versions &lt;
//        2.1.3, Redis &gt;= 2.1.3 will happily update the timeout), or the key does not exist.
func (r *Redis) Expire(key string, seconds int) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.expire(key, seconds)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ExpireAt EXPIREAT works exctly like {@link #expire(String, int) EXPIRE} but instead to get the number of
//seconds representing the Time To Live of the key as a second argument (that is a relative way
//of specifying the TTL), it takes an absolute one in the form of a UNIX timestamp (Number of
//seconds elapsed since 1 Gen 1970).
//
//EXPIREAT was introduced in order to implement the Append Only File persistence mode so that
//EXPIRE commands are automatically translated into EXPIREAT commands for the append only file.
//Of course EXPIREAT can also used by programmers that need a way to simply specify that a given
//key should expire at a given time in the future.
//
//Since Redis 2.1.3 you can update the value of the timeout of a key already having an expire
//set. It is also possible to undo the expire at all turning the key into a normal key using the
//{@link #persist(String) PERSIST} command.
//
//return Integer reply, specifically: 1: the timeout was set. 0: the timeout was not set since
//        the key already has an associated timeout (this may happen only in Redis versions &lt;
//        2.1.3, Redis &gt;= 2.1.3 will happily update the timeout), or the key does not exist.
func (r *Redis) ExpireAt(key string, unixTimeSeconds int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.expireAt(key, unixTimeSeconds)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//TTL The TTL command returns the remaining time to live in seconds of a key that has an
//{@link #expire(String, int) EXPIRE} set. This introspection capability allows a Redis client to
//check how many seconds a given key will continue to be part of the dataset.
//return Integer reply, returns the remaining time to live in seconds of a key that has an
//        EXPIRE. In Redis 2.6 or older, if the Key does not exists or does not have an
//        associated expire, -1 is returned. In Redis 2.8 or newer, if the Key does not have an
//        associated expire, -1 is returned or if the Key does not exists, -2 is returned.
func (r *Redis) TTL(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.ttl(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// PTTL Like TTL this command returns the remaining time to live of a key that has an expire set,
// with the sole difference that TTL returns the amount of remaining time in seconds while PTTL returns it in milliseconds.
//In Redis 2.6 or older the command returns -1 if the key does not exist or if the key exist but has no associated expire.
//Starting with Redis 2.8 the return value in case of error changed:
//The command returns -2 if the key does not exist.
//The command returns -1 if the key exists but has no associated expire.
//
//Integer reply: TTL in milliseconds, or a negative value in order to signal an error (see the description above).
func (r *Redis) PTTL(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.pttl(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// SetRange Overwrites part of the string stored at key, starting at the specified offset,
// for the entire length of value. If the offset is larger than the current length of the string at key,
// the string is padded with zero-bytes to make offset fit. Non-existing keys are considered as empty strings,
// so this command will make sure it holds a string large enough to be able to set value at offset.
// Note that the maximum offset that you can set is 229 -1 (536870911), as Redis Strings are limited to 512 megabytes.
// If you need to grow beyond this size, you can use multiple keys.
//
// Warning: When setting the last possible byte and the string value stored at key does not yet hold a string value,
// or holds a small string value, Redis needs to allocate all intermediate memory which can block the server for some time.
// On a 2010 MacBook Pro, setting byte number 536870911 (512MB allocation) takes ~300ms,
// setting byte number 134217728 (128MB allocation) takes ~80ms,
// setting bit number 33554432 (32MB allocation) takes ~30ms and setting bit number 8388608 (8MB allocation) takes ~8ms.
// Note that once this first allocation is done,
// subsequent calls to SETRANGE for the same key will not have the allocation overhead.
//
// Return value
// Integer reply: the length of the string after it was modified by the command.
func (r *Redis) SetRange(key string, offset int64, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.setrange(key, offset, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// GetRange Warning: this command was renamed to GETRANGE, it is called SUBSTR in Redis versions <= 2.0.
// Returns the substring of the string value stored at key,
// determined by the offsets start and end (both are inclusive).
// Negative offsets can be used in order to provide an offset starting from the end of the string.
// So -1 means the last character, -2 the penultimate and so forth.
//
// The function handles out of range requests by limiting the resulting range to the actual length of the string.
//
// Return value
// Bulk string reply
func (r *Redis) GetRange(key string, start, end int64) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.getrange(key, start, end)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//GetSet GETSET is an atomic set this value and return the old value command. Set key to the string
//value and return the old value stored at key. The string can't be longer than 1073741824 bytes (1 GB).
//
//return Bulk reply
func (r *Redis) GetSet(key, value string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.getSet(key, value)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SetNx SETNX works exactly like {@link #set(String, String) SET} with the only difference that if the
//key already exists no operation is performed. SETNX actually means "SET if Not eXists".
//
//return Integer reply, specifically: 1 if the key was set 0 if the key was not set
func (r *Redis) SetNx(key, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.setnx(key, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SetEx The command is exactly equivalent to the following group of commands:
//{@link #set(String, String) SET} + {@link #expire(String, int) EXPIRE}. The operation is atomic.
//
//return Status code reply
func (r *Redis) SetEx(key string, seconds int, value string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.setex(key, seconds, value)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//DecrBy work just like {@link #decr(String) INCR} but instead to decrement by 1 the decrement
//is integer.
//
//INCR commands are limited to 64 bit signed integers.
//
//Note: this is actually a string operation, that is, in Redis there are not "integer" types.
//Simply the string stored at the key is parsed as a base 10 64 bit signed integer, incremented,
//and then converted back as a string.
//
//return Integer reply, this commands will reply with the new value of key after the increment.
func (r *Redis) DecrBy(key string, decrement int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.decrBy(key, decrement)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Decr Decrement the number stored at key by one. If the key does not exist or contains a value of a
//wrong type, set the key to the value of "0" before to perform the decrement operation.
//
//INCR commands are limited to 64 bit signed integers.
//
//Note: this is actually a string operation, that is, in Redis there are not "integer" types.
//Simply the string stored at the key is parsed as a base 10 64 bit signed integer, incremented,
//and then converted back as a string.
//
//return Integer reply, this commands will reply with the new value of key after the increment.
func (r *Redis) Decr(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.decr(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//IncrBy work just like {@link #incr(String) INCR} but instead to increment by 1 the increment is integer.
//
//INCR commands are limited to 64 bit signed integers.
//
//Note: this is actually a string operation, that is, in Redis there are not "integer" types.
//Simply the string stored at the key is parsed as a base 10 64 bit signed integer, incremented,
//and then converted back as a string.
//
//return Integer reply, this commands will reply with the new value of key after the increment.
func (r *Redis) IncrBy(key string, increment int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.incrBy(key, increment)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//IncrByFloat commands are limited to double precision floating point values.
//
//Note: this is actually a string operation, that is, in Redis there are not "double" types.
//Simply the string stored at the key is parsed as a base double precision floating point value,
//incremented, and then converted back as a string. There is no DECRYBYFLOAT but providing a
//negative value will work as expected.
//
//return Double reply, this commands will reply with the new value of key after the increment.
func (r *Redis) IncrByFloat(key string, increment float64) (float64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.incrByFloat(key, increment)
	if err != nil {
		return 0, err
	}
	return StrToFloat64Reply(r.client.getBulkReply())
}

//Incr Increment the number stored at key by one. If the key does not exist or contains a value of a
//wrong type, set the key to the value of "0" before to perform the increment operation.
//
//INCR commands are limited to 64 bit signed integers.
//
//Note: this is actually a string operation, that is, in Redis there are not "integer" types.
//Simply the string stored at the key is parsed as a base 10 64 bit signed integer, incremented,
//and then converted back as a string.
//
//return Integer reply, this commands will reply with the new value of key after the increment.
func (r *Redis) Incr(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.incr(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Append If the key already exists and is a string, this command appends the provided value at the end
//of the string. If the key does not exist it is created and set as an empty string, so APPEND
//will be very similar to SET in this special case.
//
//Time complexity: O(1). The amortized time complexity is O(1) assuming the appended value is
//small and the already present value is of any size, since the dynamic string library used by
//Redis will double the free space available on every reallocation.
//
//return Integer reply, specifically the total length of the string after the append operation.
func (r *Redis) Append(key, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.append(key, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SubStr Return a subset of the string from offset start to offset end (both offsets are inclusive).
//Negative offsets can be used in order to provide an offset starting from the end of the string.
//So -1 means the last char, -2 the penultimate and so forth.
//
//The function handles out of range requests without raising an error, but just limiting the
//resulting range to the actual length of the string.
//
//Time complexity: O(start+n) (with start being the start index and n the total length of the
//requested range). Note that the lookup part of this command is O(1) so for small strings this
//is actually an O(1) command.
//
//return Bulk reply
func (r *Redis) SubStr(key string, start, end int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.substr(key, start, end)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//HSet Set the specified hash field to the specified value.
//
//If key does not exist, a new key holding a hash is created.
//
//return If the field already exists, and the HSET just produced an update of the value, 0 is
//        returned, otherwise if a new field is created 1 is returned.
func (r *Redis) HSet(key, field, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.hset(key, field, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//HGet If key holds a hash, retrieve the value associated to the specified field.
//
//If the field is not found or the key does not exist, a special 'nil' value is returned.
//
//return Bulk reply
func (r *Redis) HGet(key, field string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.hget(key, field)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//HSetNx Set the specified hash field to the specified value if the field not exists.
//
//return If the field already exists, 0 is returned, otherwise if a new field is created 1 is returned.
func (r *Redis) HSetNx(key, field, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.hsetnx(key, field, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//HMSet Set the respective fields to the respective values.
// HMSET replaces old values with new values.
//
//If key does not exist, a new key holding a hash is created.
//
//return Return OK or Exception if hash is empty
func (r *Redis) HMSet(key string, hash map[string]string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.hmset(key, hash)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//HMGet Retrieve the values associated to the specified fields.
//
//If some of the specified fields do not exist, nil values are returned. Non existing keys are
//considered like empty hashes.
//
//return Multi Bulk Reply specifically a list of all the values associated with the specified
//        fields, in the same order of the request.
func (r *Redis) HMGet(key string, fields ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.hmget(key, fields...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//HIncrBy Increment the number stored at field in the hash at key by value.
// If key does not exist, a new key holding a hash is created.
// If field does not exist or holds a string,
// the value is set to 0 before applying the operation.
// Since the value argument is signed you can use this command to
// perform both increments and decrements.
//
// The range of values supported by HINCRBY is limited to 64 bit signed integers.
//
// return Integer reply The new value at field after the increment operation.
func (r *Redis) HIncrBy(key, field string, value int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.hincrBy(key, field, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//HIncrByFloat Increment the number stored at field in the hash at key by a double precision floating point
//value. If key does not exist, a new key holding a hash is created. If field does not exist or
//holds a string, the value is set to 0 before applying the operation. Since the value argument
//is signed you can use this command to perform both increments and decrements.
//
//The range of values supported by HINCRBYFLOAT is limited to double precision floating point
//values.
//
//return Double precision floating point reply The new value at field after the increment
//        operation.
func (r *Redis) HIncrByFloat(key, field string, increment float64) (float64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.hincrByFloat(key, field, increment)
	if err != nil {
		return 0, err
	}
	return StrToFloat64Reply(r.client.getBulkReply())
}

//HExists Test for existence of a specified field in a hash.
//
// Return 1 if the hash stored at key contains the specified field.
// Return 0 if the key is not found or the field is not present.
func (r *Redis) HExists(key, field string) (bool, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return false, err
	}
	err = r.client.hexists(key, field)
	if err != nil {
		return false, err
	}
	return Int64ToBoolReply(r.client.getIntegerReply())
}

// HDel Remove the specified field from an hash stored at key.
//
// return If the field was present in the hash it is deleted and 1 is returned,
// otherwise 0 is returned and no operation is performed.
func (r *Redis) HDel(key string, fields ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.hdel(key, fields...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// HLen Return the number of items in a hash.
//
// return The number of entries (fields) contained in the hash stored at key.
// If the specified key does not exist, 0 is returned assuming an empty hash.
func (r *Redis) HLen(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.hlen(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//HKeys Return all the fields in a hash.
//
//return All the fields names contained into a hash.
func (r *Redis) HKeys(key string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.hkeys(key)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//HVals Return all the values in a hash.
//
//return All the fields values contained into a hash.
func (r *Redis) HVals(key string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.hvals(key)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//HGetAll Return all the fields and associated values in a hash.
//
//return All the fields and values contained into a hash.
func (r *Redis) HGetAll(key string) (map[string]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.hgetAll(key)
	if err != nil {
		return nil, err
	}
	return StrArrToMapReply(r.client.getMultiBulkReply())
}

//RPush Add the string value to the head (LPUSH) or tail (RPUSH) of the list stored at key. If the key
//does not exist an empty list is created just before the append operation. If the key exists but
//is not a List an error is returned.
//
//return Integer reply, specifically, the number of elements inside the list after the push
//        operation.
func (r *Redis) RPush(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.rpush(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//LPush Add the string value to the head (LPUSH) or tail (RPUSH) of the list stored at key. If the key
//does not exist an empty list is created just before the append operation. If the key exists but
//is not a List an error is returned.
//
//return Integer reply, specifically, the number of elements inside the list after the push
//        operation.
func (r *Redis) LPush(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.lpush(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//LLen Return the length of the list stored at the specified key. If the key does not exist zero is
//returned (the same behaviour as for empty lists). If the value stored at key is not a list an
//error is returned.
//
//return The length of the list.
func (r *Redis) LLen(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.llen(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//LRange Return the specified elements of the list stored at the specified key. Start and end are
//zero-based indexes. 0 is the first element of the list (the list head), 1 the next element and
//so on.
//
//For example LRANGE foobar 0 2 will return the first three elements of the list.
//
//start and end can also be negative numbers indicating offsets from the end of the list. For
//example -1 is the last element of the list, -2 the penultimate element and so on.
//
//<b>Consistency with range functions in various programming languages</b>
//
//Note that if you have a list of numbers from 0 to 100, LRANGE 0 10 will return 11 elements,
//that is, rightmost item is included. This may or may not be consistent with behavior of
//range-related functions in your programming language of choice (think Ruby's Range.new,
//Array#slice or Python's range() function).
//
//LRANGE behavior is consistent with one of Tcl.
//
//Indexes out of range will not produce an error: if start is over the end of the list, or start
//&gt; end, an empty list is returned. If end is over the end of the list Redis will threat it
//just like the last element of the list.
//
//return Multi bulk reply, specifically a list of elements in the specified range.
func (r *Redis) LRange(key string, start, stop int64) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.lrange(key, start, stop)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//LTrim Trim an existing list so that it will contain only the specified range of elements specified.
//Start and end are zero-based indexes. 0 is the first element of the list (the list head), 1 the
//next element and so on.
//
//For example LTRIM foobar 0 2 will modify the list stored at foobar key so that only the first
//three elements of the list will remain.
//
//start and end can also be negative numbers indicating offsets from the end of the list. For
//example -1 is the last element of the list, -2 the penultimate element and so on.
//
//Indexes out of range will not produce an error: if start is over the end of the list, or start
//&gt; end, an empty list is left as value. If end over the end of the list Redis will threat it
//just like the last element of the list.
//
//Hint: the obvious use of LTRIM is together with LPUSH/RPUSH. For example:
//
//{@code lpush("mylist", "someelement"); ltrim("mylist", 0, 99); //}
//
//The above two commands will push elements in the list taking care that the list will not grow
//without limits. This is very useful when using Redis to store logs for example. It is important
//to note that when used in this way LTRIM is an O(1) operation because in the average case just
//one element is removed from the tail of the list.
//
//return Status code reply
func (r *Redis) LTrim(key string, start, stop int64) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.ltrim(key, start, stop)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//LIndex Return the specified element of the list stored at the specified key. 0 is the first element, 1
//the second and so on. Negative indexes are supported, for example -1 is the last element, -2
//the penultimate and so on.
//
//If the value stored at key is not of list type an error is returned. If the index is out of
//range a 'nil' reply is returned.
//
//return Bulk reply, specifically the requested element
func (r *Redis) LIndex(key string, index int64) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.lindex(key, index)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//LSet Set a new value as the element at index position of the List at key.
//
//Out of range indexes will generate an error.
//
//Similarly to other list commands accepting indexes, the index can be negative to access
//elements starting from the end of the list. So -1 is the last element, -2 is the penultimate,
//and so forth.
//
//return Status code reply
func (r *Redis) LSet(key string, index int64, value string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.lset(key, index, value)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//LRem Remove the first count occurrences of the value element from the list. If count is zero all the
//elements are removed. If count is negative elements are removed from tail to head, instead to
//go from head to tail that is the normal behaviour. So for example LREM with count -2 and hello
//as value to remove against the list (a,b,c,hello,x,hello,hello) will lave the list
//(a,b,c,hello,x). The number of removed elements is returned as an integer, see below for more
//information about the returned value. Note that non existing keys are considered like empty
//lists by LREM, so LREM against non existing keys will always return 0.
//
//return Integer Reply, specifically: The number of removed elements if the operation succeeded
func (r *Redis) LRem(key string, count int64, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.lrem(key, count, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//LPop Atomically return and remove the first (LPOP) or last (RPOP) element of the list. For example
//if the list contains the elements "a","b","c" LPOP will return "a" and the list will become
//"b","c".
//
//If the key does not exist or the list is already empty the special value 'nil' is returned.
//
//return Bulk reply
func (r *Redis) LPop(key string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.lpop(key)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//RPop Atomically return and remove the first (LPOP) or last (RPOP) element of the list. For example
//if the list contains the elements "a","b","c" RPOP will return "c" and the list will become
//"a","b".
//
//If the key does not exist or the list is already empty the special value 'nil' is returned.
//
//return Bulk reply
func (r *Redis) RPop(key string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.rPop(key)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SAdd Add the specified member to the set value stored at key. If member is already a member of the
//set no operation is performed. If key does not exist a new set with the specified member as
//sole member is created. If the key exists but does not hold a set value an error is returned.
//
//return Integer reply, specifically: 1 if the new element was added 0 if the element was
//        already a member of the set
func (r *Redis) SAdd(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sAdd(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SMembers Return all the members (elements) of the set value stored at key.
//
//return Multi bulk reply
func (r *Redis) SMembers(key string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sMembers(key)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//SRem Remove the specified member from the set value stored at key. If member was not a member of the
//set no operation is performed. If key does not hold a set value an error is returned.
//
//return Integer reply, specifically: 1 if the new element was removed 0 if the new element was
//        not a member of the set
func (r *Redis) SRem(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sRem(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SPop Remove a random element from a Set returning it as return value. If the Set is empty or the key
//does not exist, a nil object is returned.
//
//The {@link #srandmember(String)} command does a similar work but the returned element is not
//removed from the Set.
//
//return Bulk reply
func (r *Redis) SPop(key string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.sPop(key)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SPopBatch remove multi random element
// see SPop(key string)
func (r *Redis) SPopBatch(key string, count int64) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sPopBatch(key, count)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//SCard Return the set cardinality (number of elements). If the key does not exist 0 is returned, like
//for empty sets.
//return Integer reply, specifically: the cardinality (number of elements) of the set as an
//        integer.
func (r *Redis) SCard(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sCard(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SIsMember Return 1 if member is a member of the set stored at key, otherwise 0 is returned.
//
//return Integer reply, specifically: 1 if the element is a member of the set 0 if the element
//        is not a member of the set OR if the key does not exist
func (r *Redis) SIsMember(key, member string) (bool, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return false, err
	}
	err = r.client.sIsMember(key, member)
	if err != nil {
		return false, err
	}
	return Int64ToBoolReply(r.client.getIntegerReply())
}

//SInter Return the members of a set resulting from the intersection of all the sets hold at the
//specified keys. Like in {@link #lrange(String, long, long) LRANGE} the result is sent to the
//client as a multi-bulk reply (see the protocol specification for more information). If just a
//single key is specified, then this command produces the same result as
//{@link #smembers(String) SMEMBERS}. Actually SMEMBERS is just syntax sugar for SINTER.
//
//Non existing keys are considered like empty sets, so if one of the keys is missing an empty set
//is returned (since the intersection with an empty set always is an empty set).
//
//return Multi bulk reply, specifically the list of common elements.
func (r *Redis) SInter(keys ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sInter(keys...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//SInterStore This commnad works exactly like {@link #sinter(String...) SINTER} but instead of being returned
//the resulting set is sotred as destKey.
//
//return Status code reply
func (r *Redis) SInterStore(destKey string, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sInterStore(destKey, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SUnion Return the members of a set resulting from the union of all the sets hold at the specified
//keys. Like in {@link #lrange(String, long, long) LRANGE} the result is sent to the client as a
//multi-bulk reply (see the protocol specification for more information). If just a single key is
//specified, then this command produces the same result as {@link #smembers(String) SMEMBERS}.
//
//Non existing keys are considered like empty sets.
//
//return Multi bulk reply, specifically the list of common elements.
func (r *Redis) SUnion(keys ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sUnion(keys...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//SUnionStore This command works exactly like {@link #sunion(String...) SUNION} but instead of being returned
//the resulting set is stored as destKey. Any existing value in destKey will be over-written.
//
//return Status code reply
func (r *Redis) SUnionStore(destKey string, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sUnionStore(destKey, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SDiff Return the difference between the Set stored at key1 and all the Sets key2, ..., keyN
//
//<b>Example:</b>
//<pre>
//key1 = [x, a, b, c]
//key2 = [c]
//key3 = [a, d]
//SDIFF key1,key2,key3 =&gt; [x, b]
//</pre>
//Non existing keys are considered like empty sets.
//
//return Return the members of a set resulting from the difference between the first set
//        provided and all the successive sets.
func (r *Redis) SDiff(keys ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sDiff(keys...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//SDiffStore This command works exactly like {@link #sdiff(String...) SDIFF} but instead of being returned
//the resulting set is stored in destKey.
//return Status code reply
func (r *Redis) SDiffStore(destKey string, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sDiffStore(destKey, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SRandMember Return a random element from a Set, without removing the element. If the Set is empty or the
//key does not exist, a nil object is returned.
//
//The SPOP command does a similar work but the returned element is popped (removed) from the Set.
//
//return Bulk reply
func (r *Redis) SRandMember(key string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.sRandMember(key)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//ZAdd Add the specified member having the specifeid score to the sorted set stored at key. If member
//is already a member of the sorted set the score is updated, and the element reinserted in the
//right position to ensure sorting. If key does not exist a new sorted set with the specified
//member as sole member is crated. If the key exists but does not hold a sorted set value an
//error is returned.
//
//The score value can be the string representation of a double precision floating point number.
//
//return Integer reply, specifically: 1 if the new element was added 0 if the element was
//        already a member of the sorted set and the score was updated
func (r *Redis) ZAdd(key string, score float64, member string, params ...*ZAddParams) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zAdd(key, score, member, params...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZRange Returns the specified range of elements in the sorted set stored at key.
// The elements are considered to be ordered from the lowest to the highest score.
// Lexicographical order is used for elements with equal score.
//See ZREVRANGE when you need the elements ordered from highest to lowest score
// (and descending lexicographical order for elements with equal score).
//Both start and stop are zero-based indexes, where 0 is the first element,
// 1 is the next element and so on. They can also be negative numbers indicating offsets from the end of the sorted set,
// with -1 being the last element of the sorted set, -2 the penultimate element and so on.
//start and stop are inclusive ranges,
// so for example ZRANGE myzset 0 1 will return both the first and the second element of the sorted set.
//Out of range indexes will not produce an error. If start is larger than the largest index in the sorted set,
// or start > stop, an empty list is returned. If stop is larger than the end of the sorted set Redis will treat it
// like it is the last element of the sorted set.
//It is possible to pass the WITHSCORES option in order to return the scores of the elements together with the elements.
// The returned list will contain value1,score1,...,valueN,scoreN instead of value1,...,valueN.
// Client libraries are free to return a more appropriate data type (suggestion: an array with (value, score) arrays/tuples).
//Return value
//Array reply: list of elements in the specified range (optionally with their scores, in case the WITHSCORES option is given).
func (r *Redis) ZRange(key string, start, stop int64) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRange(key, start, stop)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRem Remove the specified member from the sorted set value stored at key. If member was not a member
//of the set no operation is performed. If key does not not hold a set value an error is
//returned.
//
//return Integer reply, specifically: 1 if the new element was removed 0 if the new element was
//        not a member of the set
func (r *Redis) ZRem(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zRem(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZIncrBy If member already exists in the sorted set adds the increment to its score and updates the
//position of the element in the sorted set accordingly. If member does not already exist in the
//sorted set it is added with increment as score (that is, like if the previous score was
//virtually zero). If key does not exist a new sorted set with the specified member as sole
//member is crated. If the key exists but does not hold a sorted set value an error is returned.
//
//The score value can be the string representation of a double precision floating point number.
//It's possible to provide a negative value to perform a decrement.
//
//For an introduction to sorted sets check the Introduction to Redis data types page.
//
//return The new score
func (r *Redis) ZIncrBy(key string, increment float64, member string, params ...*ZAddParams) (float64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zIncrBy(key, increment, member)
	if err != nil {
		return 0, err
	}
	return StrToFloat64Reply(r.client.getBulkReply())
}

//ZRank Return the rank (or index) or member in the sorted set at key, with scores being ordered from
//low to high.
//
//When the given member does not exist in the sorted set, the special value 'nil' is returned.
//The returned rank (or index) of the member is 0-based for both commands.
//
//return Integer reply or a nil bulk reply, specifically: the rank of the element as an integer
//        reply if the element exists. A nil bulk reply if there is no such element.
func (r *Redis) ZRank(key, member string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zRank(key, member)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZRevRank Return the rank (or index) or member in the sorted set at key, with scores being ordered from
//high to low.
//
//When the given member does not exist in the sorted set, the special value 'nil' is returned.
//The returned rank (or index) of the member is 0-based for both commands.
//
//return Integer reply or a nil bulk reply, specifically: the rank of the element as an integer
//        reply if the element exists. A nil bulk reply if there is no such element.
func (r *Redis) ZRevRank(key, member string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zRevRank(key, member)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZRevRange Returns the specified range of elements in the sorted set stored at key.
// The elements are considered to be ordered from the highest to the lowest score.
// Descending lexicographical order is used for elements with equal score.
//Apart from the reversed ordering, ZREVRANGE is similar to ZRANGE.
//Return value
//Array reply: list of elements in the specified range (optionally with their scores).
func (r *Redis) ZRevRange(key string, start, stop int64) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRevRange(key, start, stop)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZCard Return the sorted set cardinality (number of elements). If the key does not exist 0 is
//returned, like for empty sorted sets.
//
//return the cardinality (number of elements) of the set as an integer.
func (r *Redis) ZCard(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zCard(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZScore Return the score of the specified element of the sorted set at key. If the specified element
//does not exist in the sorted set, or the key does not exist at all, a special 'nil' value is
//returned.
//
//return the score
func (r *Redis) ZScore(key, member string) (float64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zScore(key, member)
	if err != nil {
		return 0, err
	}
	return StrToFloat64Reply(r.client.getBulkReply())
}

//Watch Marks the given keys to be watched for conditional execution of a transaction.
//
//Return value
//Simple string reply: always OK.
func (r *Redis) Watch(keys ...string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.watch(keys...)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Sort a Set or a List.
//
//Sort the elements contained in the List, Set, or Sorted Set value at key. By default sorting is
//numeric with elements being compared as double precision floating point numbers. This is the
//simplest form of SORT.
//return Assuming the Set/List at key contains a list of numbers, the return value will be the
//        list of numbers ordered from the smallest to the biggest number.
func (r *Redis) Sort(key string, params ...*SortParams) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sort(key, params...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

// ZCount Returns the number of elements in the sorted set at key with a score between min and max.
// The min and max arguments have the same semantic as described for ZRANGEBYSCORE.
// Note: the command has a complexity of just O(log(N))
// because it uses elements ranks (see ZRANK) to get an idea of the range.
// Because of this there is no need to do a work proportional to the size of the range.
//
// Return value
// Integer reply: the number of elements in the specified score range.
func (r *Redis) ZCount(key string, min, max float64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zCount(key, min, max)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZRangeByScore Return the all the elements in the sorted set at key with a score between min and max
//(including elements with score equal to min or max).
//
//The elements having the same score are returned sorted lexicographically as ASCII strings (this
//follows from a property of Redis sorted sets and does not involve further computation).
//
//Using the optional {@link #zrangeByScore(String, double, double, int, int) LIMIT} it's possible
//to get only a range of the matching elements in an SQL-alike way. Note that if offset is large
//the commands needs to traverse the list for offset elements and this adds up to the O(M)
//figure.
//
//The {@link #zcount(String, double, double) ZCOUNT} command is similar to
//{@link #zrangeByScore(String, double, double) ZRANGEBYSCORE} but instead of returning the
//actual elements in the specified interval, it just returns the number of matching elements.
//
//<b>Exclusive intervals and infinity</b>
//
//min and max can be -inf and +inf, so that you are not required to know what's the greatest or
//smallest element in order to take, for instance, elements "up to a given value".
//
//Also while the interval is for default closed (inclusive) it's possible to specify open
//intervals prefixing the score with a "(" character, so for instance:
//
//{@code ZRANGEBYSCORE zset (1.3 5}
//
//Will return all the values with score &gt; 1.3 and &lt;= 5, while for instance:
//
//{@code ZRANGEBYSCORE zset (5 (10}
//
//Will return all the values with score &gt; 5 and &lt; 10 (5 and 10 excluded).
//
//param min a double or Double.MIN_VALUE for "-inf"
//param max a double or Double.MAX_VALUE for "+inf"
//return Multi bulk reply specifically a list of elements in the specified score range.
func (r *Redis) ZRangeByScore(key string, min, max float64) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRangeByScore(key, min, max)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRangeByScoreWithScores Return the all the elements in the sorted set at key with a score between min and max
//(including elements with score equal to min or max).
//
//The elements having the same score are returned sorted lexicographically as ASCII strings (this
//follows from a property of Redis sorted sets and does not involve further computation).
//
//Using the optional {@link #zrangeByScore(String, double, double, int, int) LIMIT} it's possible
//to get only a range of the matching elements in an SQL-alike way. Note that if offset is large
//the commands needs to traverse the list for offset elements and this adds up to the O(M)
//figure.
//
//The {@link #zcount(String, double, double) ZCOUNT} command is similar to
//{@link #zrangeByScore(String, double, double) ZRANGEBYSCORE} but instead of returning the
//actual elements in the specified interval, it just returns the number of matching elements.
//
//<b>Exclusive intervals and infinity</b>
//
//min and max can be -inf and +inf, so that you are not required to know what's the greatest or
//smallest element in order to take, for instance, elements "up to a given value".
//
//Also while the interval is for default closed (inclusive) it's possible to specify open
//intervals prefixing the score with a "(" character, so for instance:
//
//{@code ZRANGEBYSCORE zset (1.3 5}
//
//Will return all the values with score &gt; 1.3 and &lt;= 5, while for instance:
//
//{@code ZRANGEBYSCORE zset (5 (10}
//
//Will return all the values with score &gt; 5 and &lt; 10 (5 and 10 excluded).
//
//return Multi bulk reply specifically a list of elements in the specified score range.
func (r *Redis) ZRangeByScoreWithScores(key string, min, max float64) ([]Tuple, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRangeByScoreWithScores(key, min, max)
	if err != nil {
		return nil, err
	}
	return StrArrToTupleReply(r.client.getMultiBulkReply())
}

// ZRevRangeByScore Returns all the elements in the sorted set at key with a score between max and min
// (including elements with score equal to max or min). In contrary to the default ordering of sorted sets,
// for this command the elements are considered to be ordered from high to low scores.
// The elements having the same score are returned in reverse lexicographical order.
// Apart from the reversed ordering, ZREVRANGEBYSCORE is similar to ZRANGEBYSCORE.
//
// Return value
// Array reply: list of elements in the specified score range (optionally with their scores).
func (r *Redis) ZRevRangeByScore(key string, max, min float64) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRevRangeByScore(key, max, min)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRevRangeByScoreWithScores see ZRevRangeByScore(key, max, min string)
func (r *Redis) ZRevRangeByScoreWithScores(key string, max, min float64) ([]Tuple, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRevRangeByScoreWithScores(key, max, min)
	if err != nil {
		return nil, err
	}
	return StrArrToTupleReply(r.client.getMultiBulkReply())
}

//ZRemRangeByRank Remove all elements in the sorted set at key with rank between start and end. Start and end are
//0-based with rank 0 being the element with the lowest score. Both start and end can be negative
//numbers, where they indicate offsets starting at the element with the highest rank. For
//example: -1 is the element with the highest score, -2 the element with the second highest score
//and so forth.
func (r *Redis) ZRemRangeByRank(key string, start, stop int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zRemRangeByRank(key, start, stop)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// StrLen Returns the length of the string value stored at key.
// An error is returned when key holds a non-string value.
// Return value
// Integer reply: the length of the string at key, or 0 when key does not exist.
func (r *Redis) StrLen(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.strLen(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// LPushX Inserts value at the head of the list stored at key,
// only if key already exists and holds a list. In contrary to LPUSH,
// no operation will be performed when key does not yet exist.
// Return value
// Integer reply: the length of the list after the push operation.
func (r *Redis) LPushX(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.lPushX(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Persist Undo a {@link #expire(String, int) expire} at turning the expire key into a normal key.
//
//return Integer reply, specifically: 1: the key is now persist. 0: the key is not persist (only
//        happens when key not set).
func (r *Redis) Persist(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.persist(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// RPushX Inserts value at the tail of the list stored at key,
// only if key already exists and holds a list. In contrary to RPUSH,
// no operation will be performed when key does not yet exist.
//
// Return value
// Integer reply: the length of the list after the push operation.
func (r *Redis) RPushX(key string, members ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.rPushX(key, members...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// Echo Returns message.
//
// Return value
// Bulk string reply
func (r *Redis) Echo(string string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.echo(string)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SetWithParams see SetWithParamsAndTime(key, value, nxxx, expx string, time int64)
func (r *Redis) SetWithParams(key, value, nxxx string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.setWithParams(key, value, nxxx)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//PExpire This command works exactly like EXPIRE but the time to live of the key is specified in milliseconds instead of seconds.
//
//Return value
//Integer reply, specifically:
//
//1 if the timeout was set.
//0 if key does not exist.
func (r *Redis) PExpire(key string, milliseconds int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.pExpire(key, milliseconds)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//PExpireAt has the same effect and semantic as EXPIREAT,
// but the Unix time at which the key will expire is specified in milliseconds instead of seconds.
//
//Return value
//Integer reply, specifically:
//
//1 if the timeout was set.
//0 if key does not exist.
func (r *Redis) PExpireAt(key string, millisecondsTimestamp int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.pExpireAt(key, millisecondsTimestamp)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SetBitWithBool see SetBit(key string, offset int64, value string)
func (r *Redis) SetBitWithBool(key string, offset int64, value bool) (bool, error) {
	var valueByte []byte
	if value {
		valueByte = bytesTrue
	} else {
		valueByte = bytesFalse
	}
	return r.SetBit(key, offset, string(valueByte))
}

//SetBit Sets or clears the bit at offset in the string value stored at key.
//
//The bit is either set or cleared depending on value, which can be either 0 or 1.
// When key does not exist, a new string value is created.
// The string is grown to make sure it can hold a bit at offset.
// The offset argument is required to be greater than or equal to 0,
// and smaller than 232 (this limits bitmaps to 512MB). When the string at key is grown, added bits are set to 0.
//
//Warning: When setting the last possible bit (offset equal to 232 -1) and
// the string value stored at key does not yet hold a string value,
// or holds a small string value,
// Redis needs to allocate all intermediate memory which can block the server for some time.
// On a 2010 MacBook Pro, setting bit number 232 -1 (512MB allocation) takes ~300ms,
// setting bit number 230 -1 (128MB allocation) takes ~80ms,
// setting bit number 228 -1 (32MB allocation) takes ~30ms and setting bit number 226 -1 (8MB allocation) takes ~8ms.
// Note that once this first allocation is done,
// subsequent calls to SETBIT for the same key will not have the allocation overhead.
//
//Return value
//Integer reply: the original bit value stored at offset.
func (r *Redis) SetBit(key string, offset int64, value string) (bool, error) {
	err := r.client.setBit(key, offset, value)
	if err != nil {
		return false, err
	}
	return Int64ToBoolReply(r.client.getIntegerReply())
}

//GetBit Returns the bit value at offset in the string value stored at key.
//
//When offset is beyond the string length,
// the string is assumed to be a contiguous space with 0 bits.
// When key does not exist it is assumed to be an empty string,
// so offset is always out of range and the value is also assumed to be a contiguous space with 0 bits.
//
//Return value
//Integer reply: the bit value stored at offset.
func (r *Redis) GetBit(key string, offset int64) (bool, error) {
	err := r.client.getBit(key, offset)
	if err != nil {
		return false, err
	}
	return Int64ToBoolReply(r.client.getIntegerReply())
}

// PSetEx works exactly like SETEX with the sole difference that the expire time is specified in milliseconds instead of seconds.
func (r *Redis) PSetEx(key string, milliseconds int64, value string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.pSetEx(key, milliseconds, value)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SRandMemberBatch see SRandMember(key string)
func (r *Redis) SRandMemberBatch(key string, count int) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sRandMemberBatch(key, count)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZAddByMap see ZAdd(key string, score float64, member string, mparams ...ZAddParams)
func (r *Redis) ZAddByMap(key string, scoreMembers map[string]float64, params ...*ZAddParams) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.ZAddByMap(key, scoreMembers, params...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZRangeWithScores see ZRange()
func (r *Redis) ZRangeWithScores(key string, start, end int64) ([]Tuple, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.ZRangeWithScores(key, start, end)
	if err != nil {
		return nil, err
	}
	return StrArrToTupleReply(r.client.getMultiBulkReply())
}

//ZRevRangeWithScores see ZRevRange()
func (r *Redis) ZRevRangeWithScores(key string, start, end int64) ([]Tuple, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.ZRevRangeWithScores(key, start, end)
	if err != nil {
		return nil, err
	}
	return StrArrToTupleReply(r.client.getMultiBulkReply())
}

//ZRangeByScoreBatch see ZRange()
func (r *Redis) ZRangeByScoreBatch(key string, min, max float64, offset, count int) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRangeByScoreBatch(key, min, max, offset, count)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRangeByScoreWithScoresBatch see ZRange()
func (r *Redis) ZRangeByScoreWithScoresBatch(key string, min, max float64, offset, count int) ([]Tuple, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRangeByScoreWithScoresBatch(key, min, max, offset, count)
	if err != nil {
		return nil, err
	}
	return StrArrToTupleReply(r.client.getMultiBulkReply())
}

//ZRevRangeByScoreWithScoresBatch see ZRevRange()
func (r *Redis) ZRevRangeByScoreWithScoresBatch(key string, max, min float64, offset, count int) ([]Tuple, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zRevRangeByScoreWithScoresBatch(key, max, min, offset, count)
	if err != nil {
		return nil, err
	}
	return StrArrToTupleReply(r.client.getMultiBulkReply())
}

//ZRemRangeByScore Removes all elements in the sorted set stored at key with a score between min and max (inclusive).
//
//Since version 2.1.6, min and max can be exclusive, following the syntax of ZRANGEBYSCORE.
//
//Return value
//Integer reply: the number of elements removed.
func (r *Redis) ZRemRangeByScore(key string, min, max float64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.ZRemRangeByScore(key, min, max)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZLexCount When all the elements in a sorted set are inserted with the same score,
// in order to force lexicographical ordering,
// this command returns the number of elements in the sorted set at key with a value between min and max.
//
//The min and max arguments have the same meaning as described for ZRANGEBYLEX.
//
//Return value
//Integer reply: the number of elements in the specified score range.
func (r *Redis) ZLexCount(key, min, max string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zlexcount(key, min, max)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZRangeByLex When all the elements in a sorted set are inserted with the same score,
// in order to force lexicographical ordering,
// this command returns all the elements in the sorted set at key with a value between min and max.
//
//If the elements in the sorted set have different scores, the returned elements are unspecified.
//
//The elements are considered to be ordered from lower to higher strings as compared byte-by-byte
// using the memcmp() C function. Longer strings are considered greater than shorter strings if the common part is identical.
//
//The optional LIMIT argument can be used to only get a range of the matching elements
// (similar to SELECT LIMIT offset, count in SQL).
// A negative count returns all elements from the offset.
// Keep in mind that if offset is large, the sorted set needs to be traversed
// for offset elements before getting to the elements to return, which can add up to O(N) time complexity.
//Return value
//Array reply: list of elements in the specified score range.
func (r *Redis) ZRangeByLex(key, min, max string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zrangeByLex(key, min, max)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRangeByLexBatch see ZRangeByLex()
func (r *Redis) ZRangeByLexBatch(key, min, max string, offset, count int) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zrangeByLexBatch(key, min, max, offset, count)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRevRangeByLex When all the elements in a sorted set are inserted with the same score,
// in order to force lexicographical ordering,
// this command returns all the elements in the sorted set at key with a value between max and min.
//
//Apart from the reversed ordering, ZREVRANGEBYLEX is similar to ZRANGEBYLEX.
//
//Return value
//Array reply: list of elements in the specified score range.
func (r *Redis) ZRevRangeByLex(key, max, min string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zrevrangeByLex(key, max, min)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRevRangeByLexBatch see ZRevRangeByLex()
func (r *Redis) ZRevRangeByLexBatch(key, max, min string, offset, count int) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zrevrangeByLexBatch(key, max, min, offset, count)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ZRemRangeByLex When all the elements in a sorted set are inserted with the same score,
// in order to force lexicographical ordering,
// this command removes all elements in the sorted set stored at key
// between the lexicographical range specified by min and max.
//
//The meaning of min and max are the same of the ZRANGEBYLEX command.
// Similarly, this command actually returns the same elements that ZRANGEBYLEX would return
// if called with the same min and max arguments.
//
//Return value
//Integer reply: the number of elements removed.
func (r *Redis) ZRemRangeByLex(key, min, max string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zremrangeByLex(key, min, max)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//LInsert Inserts value in the list stored at key either before or after the reference value pivot.
//When key does not exist, it is considered an empty list and no operation is performed.
//An error is returned when key exists but does not hold a list value.
//
//Return value
//Integer reply: the length of the list after the insert operation, or -1 when the value pivot was not found.
func (r *Redis) LInsert(key string, where *ListOption, pivot, value string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.linsert(key, where, pivot, value)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Move key from the currently selected database (see SELECT) to the specified destination database.
// When key already exists in the destination database,
// or it does not exist in the source database, it does nothing.
// It is possible to use MOVE as a locking primitive because of this.
//
//Return value
//Integer reply, specifically:
//
//1 if key was moved.
//0 if key was not moved.
func (r *Redis) Move(key string, dbIndex int) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.move(key, dbIndex)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//BitCount Count the number of set bits (population counting) in a string.
//
//By default all the bytes contained in the string are examined.
// It is possible to specify the counting operation only in an interval passing the additional arguments start and end.
//
//Like for the GETRANGE command start and end can contain negative values
// in order to index bytes starting from the end of the string, where -1 is the last byte, -2 is the penultimate, and so forth.
//
//Non-existent keys are treated as empty strings, so the command will return zero.
//
//Return value
//Integer reply
//The number of bits set to 1.
func (r *Redis) BitCount(key string) (int64, error) {
	err := r.client.bitcount(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//BitCountRange see BitCount()
func (r *Redis) BitCountRange(key string, start, end int64) (int64, error) {
	err := r.client.bitcountRange(key, start, end)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//BitPos Return the position of the first bit set to 1 or 0 in a string.
func (r *Redis) BitPos(key string, value bool, params ...*BitPosParams) (int64, error) {
	err := r.client.bitpos(key, value, params...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//HScan scan keys of hash , see scan
func (r *Redis) HScan(key, cursor string, params ...*ScanParams) (*ScanResult, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.hscan(key, cursor, params...)
	if err != nil {
		return nil, err
	}
	return ObjArrToScanResultReply(r.client.getObjectMultiBulkReply())
}

//SScan scan keys of set,see scan
func (r *Redis) SScan(key, cursor string, params ...*ScanParams) (*ScanResult, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.sscan(key, cursor, params...)
	if err != nil {
		return nil, err
	}
	return ObjArrToScanResultReply(r.client.getObjectMultiBulkReply())
}

//ZScan scan keys of zset,see scan
func (r *Redis) ZScan(key, cursor string, params ...*ScanParams) (*ScanResult, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.zscan(key, cursor, params...)
	if err != nil {
		return nil, err
	}
	return ObjArrToScanResultReply(r.client.getObjectMultiBulkReply())
}

//PfAdd  Adds all the element arguments to the HyperLogLog data structure stored at the variable name specified as first argument.
//Return value
//Integer reply, specifically:
//
//1 if at least 1 HyperLogLog internal register was altered. 0 otherwise.
func (r *Redis) PfAdd(key string, elements ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.pfadd(key, elements...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//GeoAdd add geo point
func (r *Redis) GeoAdd(key string, longitude, latitude float64, member string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.geoadd(key, longitude, latitude, member)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//GeoAddByMap add geo point by map
// Return value
//Integer reply, specifically:
//The number of elements added to the sorted set,
// not including elements already existing for which the score was updated.
func (r *Redis) GeoAddByMap(key string, memberCoordinateMap map[string]GeoCoordinate) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.geoaddByMap(key, memberCoordinateMap)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//GeoDist calculate distance of geo points
func (r *Redis) GeoDist(key, member1, member2 string, unit ...*GeoUnit) (float64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.geodist(key, member1, member2, unit...)
	if err != nil {
		return 0, err
	}
	return StrToFloat64Reply(r.client.getBulkReply())
}

//GeoHash get geo point hash
func (r *Redis) GeoHash(key string, members ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.geohash(key, members...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//GeoPos get geo points
func (r *Redis) GeoPos(key string, members ...string) ([]*GeoCoordinate, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.geopos(key, members...)
	if err != nil {
		return nil, err
	}
	return ObjArrToGeoCoordinateReply(r.client.getObjectMultiBulkReply())
}

//GeoRadius get members in certain range
func (r *Redis) GeoRadius(key string, longitude, latitude, radius float64, unit *GeoUnit, param ...*GeoRadiusParams) ([]GeoRadiusResponse, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.georadius(key, longitude, latitude, radius, unit, param...)
	if err != nil {
		return nil, err
	}
	return ObjArrToGeoRadiusResponseReply(r.client.getObjectMultiBulkReply())
}

//GeoRadiusByMember get members in certain range
func (r *Redis) GeoRadiusByMember(key, member string, radius float64, unit *GeoUnit, param ...*GeoRadiusParams) ([]GeoRadiusResponse, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.georadiusByMember(key, member, radius, unit, param...)
	if err != nil {
		return nil, err
	}
	return ObjArrToGeoRadiusResponseReply(r.client.getObjectMultiBulkReply())
}

//BitField The command treats a Redis string as a array of bits,
// and is capable of addressing specific integer fields of varying bit widths and arbitrary non (necessary) aligned offset.
func (r *Redis) BitField(key string, arguments ...string) ([]int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.bitfield(key, arguments...)
	if err != nil {
		return nil, err
	}
	return r.client.getIntegerMultiBulkReply()
}

//</editor-fold>

//<editor-fold desc="multikeycommands">

//Keys Returns all the keys matching the glob-style pattern as space separated strings. For example if
//you have in the database the keys "foo" and "foobar" the command "KEYS foo*" will return
//"foo foobar".
//
//Note that while the time complexity for this operation is O(n) the constant times are pretty
//low. For example Redis running on an entry level laptop can scan a 1 million keys database in
//40 milliseconds. <b>Still it's better to consider this one of the slow commands that may ruin
//the DB performance if not used with care.</b>
//
//In other words this command is intended only for debugging and special operations like creating
//a script to change the DB schema. Don't use it in your normal code. Use Redis Sets in order to
//group together a subset of objects.
//
//Glob style patterns examples:
//<ul>
//<li>h?llo will match hello hallo hhllo
//<li>h*llo will match hllo heeeello
//<li>h[ae]llo will match hello and hallo, but not hillo
//</ul>
//
//Use \ to escape special chars if you want to match them verbatim.
//
//return Multi bulk reply
func (r *Redis) Keys(pattern string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.keys(pattern)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//Del Remove the specified keys. If a given key does not exist no operation is performed for this
//key. The command returns the number of keys removed.
//param keys
//return Integer reply, specifically: an integer greater than 0 if one or more keys were removed
//        0 if none of the specified key existed
func (r *Redis) Del(keys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.del(keys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Exists Test if the specified key exists. The command returns the number of keys existed
//param keys
//return Integer reply, specifically: an integer greater than 0 if one or more keys were removed
//        0 if none of the specified key existed
func (r *Redis) Exists(keys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.exists(keys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Rename Atomically renames the key oldKey to newKey. If the source and destination name are the same an
//error is returned. If newKey already exists it is overwritten.
//
//return Status code repy
func (r *Redis) Rename(oldKey, newKey string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.rename(oldKey, newKey)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//RenameNx Rename oldKey into newKey but fails if the destination key newKey already exists.
//
//return Integer reply, specifically: 1 if the key was renamed 0 if the target key already exist
func (r *Redis) RenameNx(oldKey, newKey string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.renamenx(oldKey, newKey)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//MGet Get the values of all the specified keys. If one or more keys dont exist or is not of type
//String, a 'nil' value is returned instead of the value of the specified key, but the operation
//never fails.
//
//return Multi bulk reply
func (r *Redis) MGet(keys ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.mget(keys...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//MSet Set the the respective keys to the respective values. MSET will replace old values with new
//values, while {@link #msetnx(String...) MSETNX} will not perform any operation at all even if
//just a single key already exists.
//
//Because of this semantic MSETNX can be used in order to set different keys representing
//different fields of an unique logic object in a way that ensures that either all the fields or
//none at all are set.
//
//Both MSET and MSETNX are atomic operations. This means that for instance if the keys A and B
//are modified, another client talking to Redis can either see the changes to both A and B at
//once, or no modification at all.
//return Status code reply Basically +OK as MSET can't fail
func (r *Redis) MSet(kvs ...string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.mset(kvs...)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//MSetNx Set the the respective keys to the respective values. {@link #mset(String...) MSET} will
//replace old values with new values, while MSETNX will not perform any operation at all even if
//just a single key already exists.
//
//Because of this semantic MSETNX can be used in order to set different keys representing
//different fields of an unique logic object in a way that ensures that either all the fields or
//none at all are set.
//
//Both MSET and MSETNX are atomic operations. This means that for instance if the keys A and B
//are modified, another client talking to Redis can either see the changes to both A and B at
//once, or no modification at all.
//return Integer reply, specifically: 1 if the all the keys were set 0 if no key was set (at
//        least one key already existed)
func (r *Redis) MSetNx(kvs ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.msetnx(kvs...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//RPopLPush Atomically return and remove the last (tail) element of the srcKey list, and push the element
//as the first (head) element of the destKey list. For example if the source list contains the
//elements "a","b","c" and the destination list contains the elements "foo","bar" after an
//RPOPLPUSH command the content of the two lists will be "a","b" and "c","foo","bar".
//
//If the key does not exist or the list is already empty the special value 'nil' is returned. If
//the srcKey and destKey are the same the operation is equivalent to removing the last element
//from the list and pusing it as first element of the list, so it's a "list rotation" command.
//
//return Bulk reply
func (r *Redis) RPopLPush(srcKey, destKey string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.rpopLpush(srcKey, destKey)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SMove Move the specifided member from the set at srcKey to the set at destKey. This operation is
//atomic, in every given moment the element will appear to be in the source or destination set
//for accessing clients.
//
//If the source set does not exist or does not contain the specified element no operation is
//performed and zero is returned, otherwise the element is removed from the source set and added
//to the destination set. On success one is returned, even if the element was already present in
//the destination set.
//
//An error is raised if the source or destination keys contain a non Set value.
//
//return Integer reply, specifically: 1 if the element was moved 0 if the element was not found
//        on the first set and no operation was performed
func (r *Redis) SMove(srcKey, destKey, member string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.smove(srcKey, destKey, member)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZUnionStore Creates a union or intersection of N sorted sets given by keys k1 through kN, and stores it at
//destKey. It is mandatory to provide the number of input keys N, before passing the input keys
//and the other (optional) arguments.
//
//As the terms imply, the {@link #zinterstore(String, String...) ZINTERSTORE} command requires an
//element to be present in each of the given inputs to be inserted in the result. The
//{@link #zunionstore(String, String...) ZUNIONSTORE} command inserts all elements across all
//inputs.
//
//Using the WEIGHTS option, it is possible to add weight to each input sorted set. This means
//that the score of each element in the sorted set is first multiplied by this weight before
//being passed to the aggregation. When this option is not given, all weights default to 1.
//
//With the AGGREGATE option, it's possible to specify how the results of the union or
//intersection are aggregated. This option defaults to SUM, where the score of an element is
//summed across the inputs where it exists. When this option is set to be either MIN or MAX, the
//resulting set will contain the minimum or maximum score of an element across the inputs where
//it exists.
//
//return Integer reply, specifically the number of elements in the sorted set at destKey
func (r *Redis) ZUnionStore(destKey string, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zunionstore(destKey, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZInterStore Creates a union or intersection of N sorted sets given by keys k1 through kN, and stores it at
//destKey. It is mandatory to provide the number of input keys N, before passing the input keys
//and the other (optional) arguments.
//
//As the terms imply, the {@link #zinterstore(String, String...) ZINTERSTORE} command requires an
//element to be present in each of the given inputs to be inserted in the result. The
//{@link #zunionstore(String, String...) ZUNIONSTORE} command inserts all elements across all
//inputs.
//
//Using the WEIGHTS option, it is possible to add weight to each input sorted set. This means
//that the score of each element in the sorted set is first multiplied by this weight before
//being passed to the aggregation. When this option is not given, all weights default to 1.
//
//With the AGGREGATE option, it's possible to specify how the results of the union or
//intersection are aggregated. This option defaults to SUM, where the score of an element is
//summed across the inputs where it exists. When this option is set to be either MIN or MAX, the
//resulting set will contain the minimum or maximum score of an element across the inputs where
//it exists.
//
//return Integer reply, specifically the number of elements in the sorted set at destKey
func (r *Redis) ZInterStore(destKey string, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zinterstore(destKey, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//BLPopTimeout ...
func (r *Redis) BLPopTimeout(timeout int, keys ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.blpopTimout(timeout, keys...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//BRPopTimeout ...
func (r *Redis) BRPopTimeout(timeout int, keys ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.brpopTimout(timeout, keys...)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//BLPop BLPOP (and BRPOP) is a blocking list pop primitive. You can see this commands as blocking
//versions of LPOP and RPOP able to block if the specified keys don't exist or contain empty
//lists.
//
//The following is a description of the exact semantic. We describe BLPOP but the two commands
//are identical, the only difference is that BLPOP pops the element from the left (head) of the
//list, and BRPOP pops from the right (tail).
//
//<b>Non blocking behavior</b>
//
//When BLPOP is called, if at least one of the specified keys contain a non empty list, an
//element is popped from the head of the list and returned to the caller together with the name
//of the key (BLPOP returns a two elements array, the first element is the key, the second the
//popped value).
//
//Keys are scanned from left to right, so for instance if you issue BLPOP list1 list2 list3 0
//against a dataset where list1 does not exist but list2 and list3 contain non empty lists, BLPOP
//guarantees to return an element from the list stored at list2 (since it is the first non empty
//list starting from the left).
//
//<b>Blocking behavior</b>
//
//If none of the specified keys exist or contain non empty lists, BLPOP blocks until some other
//client performs a LPUSH or an RPUSH operation against one of the lists.
//
//Once new data is present on one of the lists, the client finally returns with the name of the
//key unblocking it and the popped value.
//
//When blocking, if a non-zero timeout is specified, the client will unblock returning a nil
//special value if the specified amount of seconds passed without a push operation against at
//least one of the specified keys.
//
//The timeout argument is interpreted as an integer value. A timeout of zero means instead to
//block forever.
//
//<b>Multiple clients blocking for the same keys</b>
//
//Multiple clients can block for the same key. They are put into a queue, so the first to be
//served will be the one that started to wait earlier, in a first-blpopping first-served fashion.
//
//<b>blocking POP inside a MULTI/EXEC transaction</b>
//
//BLPOP and BRPOP can be used with pipelining (sending multiple commands and reading the replies
//in batch), but it does not make sense to use BLPOP or BRPOP inside a MULTI/EXEC block (a Redis
//transaction).
//
//The behavior of BLPOP inside MULTI/EXEC when the list is empty is to return a multi-bulk nil
//reply, exactly what happens when the timeout is reached. If you like science fiction, think at
//it like if inside MULTI/EXEC the time will flow at infinite speed :)
//
//return BLPOP returns a two-elements array via a multi bulk reply in order to return both the
//        unblocking key and the popped value.
//
//        When a non-zero timeout is specified, and the BLPOP operation timed out, the return
//        value is a nil multi bulk reply. Most client values will return false or nil
//        accordingly to the programming language used.
func (r *Redis) BLPop(args ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.blpop(args)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//BRPop see blpop
func (r *Redis) BRPop(args ...string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.brpop(args)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//SortStore sort old sets or list,then store the result to a new set or list
func (r *Redis) SortStore(srcKey, destKey string, params ...*SortParams) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.sortMulti(srcKey, destKey, params...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Unwatch cancel all watches for keys
// always return OK
func (r *Redis) Unwatch() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.unwatch()
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//ZInterStoreWithParams ...
func (r *Redis) ZInterStoreWithParams(destKey string, params *ZParams, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zinterstoreWithParams(destKey, params, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ZUnionStoreWithParams ...
func (r *Redis) ZUnionStoreWithParams(destKey string, params *ZParams, srcKeys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.zunionstoreWithParams(destKey, params, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//BRPopLPush ...
func (r *Redis) BRPopLPush(srcKey, destKey string, timeout int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.brpoplpush(srcKey, destKey, timeout)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//Publish ...
func (r *Redis) Publish(channel, message string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.publish(channel, message)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Subscribe ...
func (r *Redis) Subscribe(redisPubSub *RedisPubSub, channels ...string) error {
	err := r.client.connection.setTimeoutInfinite()
	defer r.client.connection.rollbackTimeout()
	if err != nil {
		return err
	}
	err = redisPubSub.proceed(r, channels...)
	if err != nil {
		return err
	}
	return nil
}

//PSubscribe ...
func (r *Redis) PSubscribe(redisPubSub *RedisPubSub, patterns ...string) error {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return err
	}
	err = r.client.connection.setTimeoutInfinite()
	defer r.client.connection.rollbackTimeout()
	if err != nil {
		return err
	}
	err = redisPubSub.proceedWithPatterns(r, patterns...)
	if err != nil {
		return err
	}
	return nil
}

//RandomKey ...
func (r *Redis) RandomKey() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.randomKey()
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//BitOp ...
func (r *Redis) BitOp(op BitOP, destKey string, srcKeys ...string) (int64, error) {
	err := r.client.bitop(op, destKey, srcKeys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Scan ...
func (r *Redis) Scan(cursor string, params ...*ScanParams) (*ScanResult, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.scan(cursor, params...)
	if err != nil {
		return nil, err
	}
	return ObjArrToScanResultReply(r.client.getObjectMultiBulkReply())
}

//PfMerge Merge multiple HyperLogLog values into an unique value that will approximate the cardinality
// of the union of the observed Sets of the source HyperLogLog structures.
//
//The computed merged HyperLogLog is set to the destination variable,
// which is created if does not exist (defaulting to an empty HyperLogLog).
//
//Return value
//Simple string reply: The command just returns OK.
func (r *Redis) PfMerge(destKey string, srcKeys ...string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.pfmerge(destKey, srcKeys...)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

// PfCount When called with a single key, returns the approximated cardinality computed by
// the HyperLogLog data structure stored at the specified variable,
// which is 0 if the variable does not exist.
//Return value
//Integer reply, specifically:
//
//The approximated number of unique elements observed via PFADD.
func (r *Redis) PfCount(keys ...string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.pfcount(keys...)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//</editor-fold>

//<editor-fold desc="advancedcommands">

//ConfigGet The CONFIG GET command is used to read the configuration parameters of a running Redis server.
// Not all the configuration parameters are supported in Redis 2.4,
// while Redis 2.6 can read the whole configuration of a server using this command.
func (r *Redis) ConfigGet(pattern string) ([]string, error) {
	err := r.client.configGet(pattern)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ConfigSet The CONFIG SET command is used in order to reconfigure the server at run time
// without the need to restart Redis.
// You can change both trivial parameters or switch from one to another persistence option using this command.
func (r *Redis) ConfigSet(parameter, value string) (string, error) {
	err := r.client.configSet(parameter, value)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SlowLogReset You can reset the slow log using the SLOWLOG RESET command.
// Once deleted the information is lost forever.
func (r *Redis) SlowLogReset() (string, error) {
	err := r.client.slowlogReset()
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SlowLogLen it is possible to get just the length of the slow log using the command SLOWLOG LEN.
func (r *Redis) SlowLogLen() (int64, error) {
	err := r.client.slowlogLen()
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SlowLogGet The Redis Slow Log is a system to log queries that exceeded a specified execution time.
// The execution time does not include I/O operations like talking with the client,
// sending the reply and so forth, but just the time needed to actually execute the command
// (this is the only stage of command execution where the thread is blocked and can not serve other requests in the meantime).
func (r *Redis) SlowLogGet(entries ...int64) ([]SlowLog, error) {
	err := r.client.slowlogGet(entries...)
	if err != nil {
		return nil, err
	}
	reply, err := r.client.getObjectMultiBulkReply()
	result := make([]SlowLog, 0)
	for _, re := range reply {
		item := re.([]interface{})
		args := make([]string, 0)
		for _, a := range item[3].([]interface{}) {
			args = append(args, string(a.([]byte)))
		}
		result = append(result, SlowLog{
			id:            item[0].(int64),
			timeStamp:     item[1].(int64),
			executionTime: item[2].(int64),
			args:          args,
		})
	}
	return result, err
}

//ObjectRefCount returns the number of references of the value associated with the specified key.
// This command is mainly useful for debugging.
func (r *Redis) ObjectRefCount(str string) (int64, error) {
	err := r.client.objectRefcount(str)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ObjectEncoding returns the kind of internal representation used in order to store the value associated with a key.
func (r *Redis) ObjectEncoding(str string) (string, error) {
	err := r.client.objectEncoding(str)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//ObjectIdleTime returns the number of seconds since the object stored at the specified key is idle
// (not requested by read or write operations).
// While the value is returned in seconds the actual resolution of this timer is 10 seconds,
// but may vary in future implementations.
// This subcommand is available when maxmemory-policy is set to an LRU policy or noeviction.
func (r *Redis) ObjectIdleTime(str string) (int64, error) {
	err := r.client.objectIdletime(str)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//</editor-fold>

//<editor-fold desc="scriptcommands">

//Eval evaluate scripts using the Lua interpreter built into Redis
func (r *Redis) Eval(script string, keyCount int, params ...string) (interface{}, error) {
	err := r.client.connection.setTimeoutInfinite()
	defer r.client.connection.rollbackTimeout()
	if err != nil {
		return nil, err
	}
	err = r.client.eval(script, keyCount, params...)
	if err != nil {
		return nil, err
	}
	return ObjToEvalResult(r.client.getOne())
}

//EvalByKeyArgs evaluate scripts using the Lua interpreter built into Redis
func (r *Redis) EvalByKeyArgs(script string, keys []string, args []string) (interface{}, error) {
	err := r.client.connection.setTimeoutInfinite()
	defer r.client.connection.rollbackTimeout()
	if err != nil {
		return nil, err
	}
	params := make([]string, 0)
	params = append(params, keys...)
	params = append(params, args...)
	err = r.client.eval(script, len(keys), params...)
	if err != nil {
		return nil, err
	}
	return ObjToEvalResult(r.client.getOne())
}

//EvalSha Evaluates a script cached on the server side by its SHA1 digest.
// Scripts are cached on the server side using the SCRIPT LOAD command.
// The command is otherwise identical to EVAL.
func (r *Redis) EvalSha(sha1 string, keyCount int, params ...string) (interface{}, error) {
	err := r.client.evalsha(sha1, keyCount, params...)
	if err != nil {
		return 0, err
	}
	return ObjToEvalResult(r.client.getOne())
}

//ScriptExists Returns information about the existence of the scripts in the script cache.
//Return value
//Array reply The command returns an array of integers
// that correspond to the specified SHA1 digest arguments.
// For every corresponding SHA1 digest of a script that actually exists in the script cache,
// an 1 is returned, otherwise 0 is returned.
func (r *Redis) ScriptExists(sha1 ...string) ([]bool, error) {
	err := r.client.scriptExists(sha1...)
	if err != nil {
		return nil, err
	}
	reply, err := r.client.getIntegerMultiBulkReply()
	if err != nil {
		return nil, err
	}
	arr := make([]bool, 0)
	for _, re := range reply {
		arr = append(arr, re == 1)
	}
	return arr, nil
}

//ScriptLoad Load a script into the scripts cache, without executing it.
// After the specified command is loaded into the script cache
// it will be callable using EVALSHA with the correct SHA1 digest of the script,
// exactly like after the first successful invocation of EVAL.
//Return value
//Bulk string reply This command returns the SHA1 digest of the script added into the script cache.
func (r *Redis) ScriptLoad(script string) (string, error) {
	err := r.client.scriptLoad(script)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//</editor-fold>

//<editor-fold desc="basiccommands">

// Quit Ask the server to close the connection.
// The connection is closed as soon as all pending replies have been written to the client.
//
// Return value
// Simple string reply: always OK.
func (r *Redis) Quit() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.quit()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Ping send ping command to redis server
// if the sever is running well,then it will return PONG.
func (r *Redis) Ping() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.ping()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Select send select command to change current db,normally redis server has 16 db [0,15]
func (r *Redis) Select(index int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.selectDb(index)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//FlushDB it will clear whole keys in current db
func (r *Redis) FlushDB() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.flushDB()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//DbSize return key count of current db
func (r *Redis) DbSize() (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.dbSize()
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//FlushAll it will clear whole keys in whole db
func (r *Redis) FlushAll() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.flushAll()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Auth when server is set password,then you need to auth password.
func (r *Redis) Auth(password string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.auth(password)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Save ...
func (r *Redis) Save() (string, error) {
	err := r.client.save()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//BgSave ...
func (r *Redis) BgSave() (string, error) {
	err := r.client.bgsave()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//BgRewriteAof ...
func (r *Redis) BgRewriteAof() (string, error) {
	err := r.client.bgrewriteaof()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//LastSave ...
func (r *Redis) LastSave() (int64, error) {
	err := r.client.lastsave()
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//Shutdown ...
func (r *Redis) Shutdown() (string, error) {
	err := r.client.shutdown()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Info ...
func (r *Redis) Info(section ...string) (string, error) {
	err := r.client.info(section...)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//SlaveOf ...
func (r *Redis) SlaveOf(host string, port int) (string, error) {
	err := r.client.slaveof(host, port)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//SlaveOfNoOne ...
func (r *Redis) SlaveOfNoOne() (string, error) {
	err := r.client.slaveofNoOne()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Debug ...
func (r *Redis) Debug(params DebugParams) (string, error) {
	err := r.client.debug(params)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ConfigResetStat ...
func (r *Redis) ConfigResetStat() (string, error) {
	err := r.client.configResetStat()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//WaitReplicas Synchronous replication of Redis as described here: http://antirez.com/news/66 Since Java
// Object class has implemented "wait" method, we cannot use it, so I had to change the name of
// the method. Sorry :S
func (r *Redis) WaitReplicas(replicas int, timeout int64) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.waitReplicas(replicas, timeout)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//</editor-fold>

//<editor-fold desc="clustercommands">

//ClusterNodes Each node in a Redis Cluster has its view of the current cluster configuration,
// given by the set of known nodes, the state of the connection we have with such nodes,
// their flags, properties and assigned slots, and so forth.
func (r *Redis) ClusterNodes() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterNodes()
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//ClusterMeet ...
func (r *Redis) ClusterMeet(ip string, port int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterMeet(ip, port)
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//ClusterAddSlots ...
func (r *Redis) ClusterAddSlots(slots ...int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterAddSlots(slots...)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterDelSlots ...
func (r *Redis) ClusterDelSlots(slots ...int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterDelSlots(slots...)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterInfo ...
func (r *Redis) ClusterInfo() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterInfo()
	if err != nil {
		return "", err
	}
	return r.client.getBulkReply()
}

//ClusterGetKeysInSlot ...
func (r *Redis) ClusterGetKeysInSlot(slot int, count int) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.clusterGetKeysInSlot(slot, count)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ClusterSetSlotNode ...
func (r *Redis) ClusterSetSlotNode(slot int, nodeID string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterSetSlotNode(slot, nodeID)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterSetSlotMigrating ...
func (r *Redis) ClusterSetSlotMigrating(slot int, nodeID string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterSetSlotMigrating(slot, nodeID)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterSetSlotImporting ...
func (r *Redis) ClusterSetSlotImporting(slot int, nodeID string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterSetSlotImporting(slot, nodeID)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterSetSlotStable ...
func (r *Redis) ClusterSetSlotStable(slot int) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterSetSlotStable(slot)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterForget ...
func (r *Redis) ClusterForget(nodeID string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterForget(nodeID)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterFlushSlots ...
func (r *Redis) ClusterFlushSlots() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterFlushSlots()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterKeySlot ...
func (r *Redis) ClusterKeySlot(key string) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.clusterKeySlot(key)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

// ClusterCountKeysInSlot ...
func (r *Redis) ClusterCountKeysInSlot(slot int) (int64, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return 0, err
	}
	err = r.client.clusterCountKeysInSlot(slot)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//ClusterSaveConfig ...
func (r *Redis) ClusterSaveConfig() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterSaveConfig()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterReplicate ...
func (r *Redis) ClusterReplicate(nodeID string) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterReplicate(nodeID)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterSlaves ...
func (r *Redis) ClusterSlaves(nodeID string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.clusterSlaves(nodeID)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

//ClusterFailOver This command, that can only be sent to a Redis Cluster replica node,
// forces the replica to start a manual failover of its master instance.
func (r *Redis) ClusterFailOver() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterFailover()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//ClusterSlots ...
func (r *Redis) ClusterSlots() ([]interface{}, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.clusterSlots()
	if err != nil {
		return nil, err
	}
	return r.client.getObjectMultiBulkReply()
}

//ClusterReset ...
func (r *Redis) ClusterReset(resetType Reset) (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.clusterReset(resetType)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Readonly Enables read queries for a connection to a Redis Cluster replica node.
//Normally replica nodes will redirect clients to the authoritative master for the hash slot involved in a given command,
// however clients can use replicas in order to scale reads using the READONLY command.
//
//READONLY tells a Redis Cluster replica node that the client is willing to read possibly stale data and
// is not interested in running write queries.
//
//When the connection is in readonly mode, the cluster will send a redirection to the client
// only if the operation involves keys not served by the replica's master node. This may happen because:
//
//The client sent a command about hash slots never served by the master of this replica.
//The cluster was reconfigured (for example resharded) and the replica is no longer able to serve commands for a given hash slot.
//Return value
//Simple string reply
func (r *Redis) Readonly() (string, error) {
	err := r.client.readonly()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//</editor-fold>

//<editor-fold desc="sentinelcommands">

//SentinelMasters example:
//redis 127.0.0.1:26381&gt; sentinel masters
//1)  1) "name"
//    2) "mymaster"
//    3) "ip"
//    4) "127.0.0.1"
//    5) "port"
//    6) "6379"
//    7) "runid"
//    8) "93d4d4e6e9c06d0eea36e27f31924ac26576081d"
//    9) "flags"
//   10) "master"
//   11) "pending-commands"
//   12) "0"
//   13) "last-ok-ping-reply"
//   14) "423"
//   15) "last-ping-reply"
//   16) "423"
//   17) "info-refresh"
//   18) "6107"
//   19) "num-slaves"
//   20) "1"
//   21) "num-other-sentinels"
//   22) "2"
//   23) "quorum"
//   24) "2"
func (r *Redis) SentinelMasters() ([]map[string]string, error) {
	err := r.client.sentinelMasters()
	if err != nil {
		return nil, err
	}
	return ObjArrToMapArrayReply(r.client.getObjectMultiBulkReply())
}

//SentinelGetMasterAddrByName example:
//redis 127.0.0.1:26381&gt; sentinel get-master-addr-by-name mymaster
//1) "127.0.0.1"
//2) "6379"
//return two elements list of strings : host and port.
func (r *Redis) SentinelGetMasterAddrByName(masterName string) ([]string, error) {
	err := r.client.sentinelGetMasterAddrByName(masterName)
	if err != nil {
		return nil, err
	}
	reply, err := r.client.getObjectMultiBulkReply()
	if err != nil {
		return nil, err
	}
	addrs := make([]string, 0)
	for _, re := range reply {
		if re == nil {
			addrs = append(addrs, "")
		} else {
			addrs = append(addrs, string(re.([]byte)))
		}
	}
	return addrs, err
}

//SentinelReset example:
//redis 127.0.0.1:26381&gt; sentinel reset mymaster
//(integer) 1
func (r *Redis) SentinelReset(pattern string) (int64, error) {
	err := r.client.sentinelReset(pattern)
	if err != nil {
		return 0, err
	}
	return r.client.getIntegerReply()
}

//SentinelSlaves example:
//redis 127.0.0.1:26381&gt; sentinel slaves mymaster
//1)  1) "name"
//    2) "127.0.0.1:6380"
//    3) "ip"
//    4) "127.0.0.1"
//    5) "port"
//    6) "6380"
//    7) "runid"
//    8) "d7f6c0ca7572df9d2f33713df0dbf8c72da7c039"
//    9) "flags"
//   10) "slave"
//   11) "pending-commands"
//   12) "0"
//   13) "last-ok-ping-reply"
//   14) "47"
//   15) "last-ping-reply"
//   16) "47"
//   17) "info-refresh"
//   18) "657"
//   19) "master-link-down-time"
//   20) "0"
//   21) "master-link-status"
//   22) "ok"
//   23) "master-host"
//   24) "localhost"
//   25) "master-port"
//   26) "6379"
//   27) "slave-priority"
//   28) "100"
func (r *Redis) SentinelSlaves(masterName string) ([]map[string]string, error) {
	err := r.client.sentinelSlaves(masterName)
	if err != nil {
		return nil, err
	}
	return ObjArrToMapArrayReply(r.client.getObjectMultiBulkReply())
}

//SentinelFailOver ...
func (r *Redis) SentinelFailOver(masterName string) (string, error) {
	err := r.client.sentinelFailover(masterName)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

// SentinelMonitor ...
func (r *Redis) SentinelMonitor(masterName, ip string, port, quorum int) (string, error) {
	err := r.client.sentinelMonitor(masterName, ip, port, quorum)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

// SentinelRemove ...
func (r *Redis) SentinelRemove(masterName string) (string, error) {
	err := r.client.sentinelRemove(masterName)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

// SentinelSet ...
func (r *Redis) SentinelSet(masterName string, parameterMap map[string]string) (string, error) {
	err := r.client.sentinelSet(masterName, parameterMap)
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//</editor-fold>

//<editor-fold desc="other commands">

// PubSubChannels ...
func (r *Redis) PubSubChannels(pattern string) ([]string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return nil, err
	}
	err = r.client.pubsubChannels(pattern)
	if err != nil {
		return nil, err
	}
	return r.client.getMultiBulkReply()
}

// Asking ...
func (r *Redis) Asking() (string, error) {
	err := r.checkIsInMultiOrPipeline()
	if err != nil {
		return "", err
	}
	err = r.client.asking()
	if err != nil {
		return "", err
	}
	return r.client.getStatusCodeReply()
}

//Multi get transaction of redis client ,when use transaction mode, you need to invoke this first
func (r *Redis) Multi() (*Transaction, error) {
	err := r.client.multi()
	if err != nil {
		return nil, err
	}
	return newTransaction(r.client), nil
}

//Pipelined get pipeline of redis client ,when use pipeline mode, you need to invoke this first
func (r *Redis) Pipelined() *Pipeline {
	return newPipeline(r.client)
}

//</editor-fold>
