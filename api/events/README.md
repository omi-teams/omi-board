# WebSocket event schemas

`envelope.schema.json` uses JSON Schema draft 2020-12 for the shared event
envelope. `seq` is an increasing sequence number within a connection; `type`
identifies the event. `project_id` and `ticket_id` are optional context and
`data` holds the event-specific object. No WebSocket endpoint is implemented yet.
Future event payload schemas belong here and should be referenced by event type.
