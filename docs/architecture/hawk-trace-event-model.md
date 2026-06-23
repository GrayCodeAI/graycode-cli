# Hawk Trace Event Model

## Goal

`trace` should capture enough structured information to support:

- replay
- audit
- debugging
- future hosted/team observability

## Event categories

### Session events
- session started
- session resumed
- session ended
- mode changed

### Model/runtime events
- provider selected
- model selected
- turn started
- turn completed
- streaming started/stopped
- runtime error

### Tool events
- tool requested
- tool approved/denied
- tool started
- tool completed
- tool failed

### File/system events
- file read
- file edited
- command executed
- sandbox decision

### Review/verify events
- review started
- review completed
- verify started
- verify completed
- findings emitted

## Minimum event fields

- event id
- session id
- timestamp
- actor
- component
- event type
- status
- correlation id
- payload metadata

## Design rules

- trace should capture metadata and references, not dump unnecessary raw secrets
- trace should be structured first, human-readable second
- event types should be stable enough for replay and future dashboards
- review and verification events should use the same contract vocabulary as their result objects

## Near-term use

In the local CLI world, trace primarily supports:

- debugging
- reproduction
- replay
- audit of what Hawk changed and why

## Long-term use

This same event model can later support:

- hosted dashboards
- enterprise audit logs
- team usage analytics
- policy enforcement evidence
