# db-read-write-consistency Specification (Delta)

## Purpose

Under primary/replica split (`database.read.enabled: true`, routed by `gorm.io/plugin/dbresolver`), a read may be served by a replica that has not yet applied a write committed moments earlier. That is an acceptable trade for ordinary reads, but not for reads whose result *gates a write*. This capability defines the invariant that separates the two, the explicit channels that make the intent visible at the call site, and the way the invariant is verified deterministically.

## ADDED Requirements

### Requirement: Explicit read and write connection channels

The repository layer MUST expose request-scoped accessors for an explicit write channel (always primary) and an explicit read channel (always replica), in addition to the existing default accessor, whose automatic routing MUST remain unchanged. Callers MUST NOT rely on a statement's type alone to decide which channel it lands on.

#### Scenario: Write channel bypasses replicas

- **WHEN** a caller issues a `SELECT` through the write channel while read replicas are registered
- **THEN** the statement is served by the primary connection, not by any replica

#### Scenario: Read channel targets a replica

- **WHEN** a caller issues a `SELECT` through the read channel while read replicas are registered
- **THEN** the statement is served by a replica connection, not by the primary

#### Scenario: Default channel routing is unchanged

- **WHEN** a caller issues a statement through the default accessor
- **THEN** the routing follows the pre-existing rule: writes to the primary, non-transactional reads to a replica

#### Scenario: Channels are request-scoped

- **WHEN** two concurrent requests each obtain a channel
- **THEN** each channel is bound to its own request context, so that cancelling one request MUST NOT affect the other

### Requirement: Decision reads MUST be served by the primary

A *decision read* is any read whose result gates a subsequent write — existence checks, parent/child or ancestor guards, uniqueness pre-checks, cycle detection, and state reads preceding a conditional update. Every decision read MUST be served by the primary. The result MUST reflect all writes committed before the read, regardless of replica replication lag.

#### Scenario: Child guard observes a just-committed child deletion

- **WHEN** request A soft-deletes a rule's only child, and request B then deletes the parent with the child guard enabled
- **THEN** request B succeeds, because the guard reads the primary and observes the committed deletion
- **AND** the guard MUST NOT observe a pre-deletion replica snapshot

#### Scenario: Existence check observes a just-committed insert

- **WHEN** request A creates a rule, and request B immediately posts a new rule whose uniqueness pre-check overlaps request A's insert
- **THEN** request B's pre-check observes request A's committed row

#### Scenario: Deletion guard refuses only on committed children

- **WHEN** a rule has a child that exists on the primary, and the guard read races with replication
- **THEN** the deletion is refused with the has-children error, and the parent row MUST remain

#### Scenario: Replication lag does not change the decision

- **WHEN** the replica's replication thread is stopped, so its lag is unbounded
- **THEN** every decision read still returns the value committed on the primary

### Requirement: Guard decisions SHOULD be pushed into the database

Where a guard is a predicate over the same rows the write touches, the system SHOULD express it as a single conditional write — an `UPDATE` carrying the predicate, or a transaction-scoped locked read — so the database serialises the decision instead of the application. A zero affected-row count MUST be treated as a decision failure, not silently ignored, and MUST be distinguished from a genuine write error.

#### Scenario: Conditional write replaces read-then-write

- **WHEN** a deletion is guarded by a child-existence predicate
- **THEN** the predicate is evaluated by the database as part of the same statement or transaction as the deletion
- **AND** no separate application-level pre-read is required for correctness

#### Scenario: Zero affected rows is a decision failure

- **WHEN** a conditional write matches no row because a competing request changed the state first
- **THEN** the caller receives a distinguishable decision error rather than a success

#### Scenario: Guards that cannot be expressed as a conditional write

- **WHEN** a guard requires traversing a chain that a single statement cannot express (for example ancestor-chain cycle detection)
- **THEN** the guard MAY remain in the application layer
- **AND** it MUST still be served by the primary channel

### Requirement: Stale-tolerant reads MUST keep targeting replicas

Reads whose result gates no write MUST continue to be served by replicas, and the set of such read paths MUST be explicitly enumerated and marked at the call site. The system MUST NOT satisfy this capability by routing every read to the primary.

#### Scenario: Listing reads stay on replicas

- **WHEN** a list, detail, feed, or aggregate read is issued on a path whose result gates no write
- **THEN** it is served by a replica

#### Scenario: Enumerated categories are documented

- **WHEN** a reader inspects the repository layer
- **THEN** every path permitted to read from a replica is identifiable from an explicit read-channel usage or from the enumerated category list, without having to infer routing from statement type

#### Scenario: No blanket primary routing

- **WHEN** the implementation is reviewed for this capability
- **THEN** reads that gate no write MUST NOT have been converted to primary reads

### Requirement: Replication lag MUST be injectable for verification

The system MUST provide a deterministic way to cause a replica to lag without bound, so that consistency defects fail deterministically instead of flaking. The harness MUST first confirm that replication is currently healthy, and MUST restore replication after the test. The harness MUST be confined to integration-tagged tests and MUST NOT affect ordinary unit test runs.

#### Scenario: Lag injection makes the defect deterministic

- **WHEN** the replica's replication thread is stopped and a decision-read scenario is executed against the pre-fix implementation
- **THEN** the scenario fails
- **AND** the same scenario passes once decision reads are served by the primary

#### Scenario: Harness asserts healthy replication first

- **WHEN** the harness stops the replication thread
- **THEN** it MUST have verified beforehand that the replica was actively replicating, so that an already-broken replica cannot make the test pass spuriously

#### Scenario: Harness restores replication

- **WHEN** the harness finishes, whether the test passed or failed
- **THEN** the replication thread is restarted, so subsequent tests are unaffected

#### Scenario: Lag is not inferred from replication delay metrics

- **WHEN** the harness decides whether a replica is lagging
- **THEN** it MUST NOT rely on a second-granularity delay metric, which cannot represent a sub-second window
