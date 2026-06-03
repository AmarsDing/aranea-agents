## ADDED Requirements

### Requirement: Bucket lastAccess tracking
Each throttled bucket SHALL track its last access timestamp. The lastAccess field SHALL be updated atomically on every shouldThrottle call.

#### Scenario: lastAccess updated on access
- **WHEN** shouldThrottle("chat.llm.invoke") is called
- **THEN** the bucket for the matching rule SHALL have its lastAccess set to the current unix timestamp

#### Scenario: lastAccess not updated for empty stepID
- **WHEN** shouldThrottle("") is called
- **THEN** no bucket SHALL be accessed or created

### Requirement: Background TTL eviction with lifecycle management
A background goroutine SHALL periodically scan all buckets and evict those not accessed within the TTL window. Default TTL SHALL be 30 minutes. Scan interval SHALL be 5 minutes. The goroutine SHALL have explicit Start/Stop lifecycle management controlled by Pipeline.

#### Scenario: Bucket evicted after TTL
- **WHEN** a bucket was last accessed 35 minutes ago
- **AND** TTL is 30 minutes
- **THEN** the bucket SHALL be removed from the map

#### Scenario: Bucket retained within TTL
- **WHEN** a bucket was last accessed 15 minutes ago
- **AND** TTL is 30 minutes
- **THEN** the bucket SHALL remain in the map

#### Scenario: Background scan continues after eviction
- **WHEN** the background goroutine evicts 10 buckets
- **THEN** the goroutine SHALL continue running
- **AND** the next scan SHALL proceed normally

#### Scenario: Start begins the eviction goroutine
- **WHEN** stepThrottler.Start() is called
- **THEN** the background eviction goroutine SHALL begin running
- **AND** it SHALL scan at the configured interval

#### Scenario: Stop terminates the eviction goroutine
- **WHEN** stepThrottler.Stop() is called
- **THEN** the background goroutine SHALL exit gracefully
- **AND** no further scans SHALL occur
- **AND** Stop SHALL block until the goroutine has exited

#### Scenario: Pipeline lifecycle manages throttler
- **WHEN** Pipeline.Close() is called
- **THEN** stepThrottler.Stop() SHALL be called before Pipeline returns
- **AND** no panic SHALL occur from the goroutine accessing closed resources

### Requirement: TTL configuration
The TTL duration and scan interval SHALL be configurable via ThrottleConfig.

#### Scenario: Custom TTL via config
- **WHEN** ThrottleConfig.TTL is set to 60 minutes
- **THEN** buckets not accessed for 60 minutes SHALL be evicted

#### Scenario: Default TTL when not specified
- **WHEN** ThrottleConfig.TTL is not set
- **THEN** default TTL of 30 minutes SHALL be used

### Requirement: Eviction safety
The background eviction goroutine SHALL NOT block or interfere with shouldThrottle calls. Eviction SHALL use a write lock; shouldThrottle SHALL use a read lock when possible.

#### Scenario: Concurrent eviction and throttle check
- **WHEN** the eviction goroutine is scanning buckets
- **AND** a goroutine calls shouldThrottle
- **THEN** shouldThrottle SHALL use a read lock that does not block on the eviction write lock for more than 1ms
