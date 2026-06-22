# Development

## Official SDK Commands Used

```bash
pip install -U langbot_plugin
lbp init final-review-sales-agent
lbp comp Command
lbp comp Tool
lbp comp EventListener
```

Generated Event Listener files are `components/event_listener/default.py` and `default.yaml`. LangBot official docs state one plugin can only have one Event Listener component, so the sales guard is implemented in the official default listener instead of adding a second listener.

## SDK APIs Used

- `langbot_plugin.api.definition.plugin.BasePlugin`
- `langbot_plugin.api.definition.components.command.command.Command`
- `Command.subcommand(...)`
- `langbot_plugin.api.entities.builtin.command.context.ExecuteContext`
- `langbot_plugin.api.entities.builtin.command.context.CommandReturn`
- `langbot_plugin.api.definition.components.tool.tool.Tool`
- `Tool.call(params, session, query_id)`
- `langbot_plugin.api.definition.components.common.event_listener.EventListener`
- `EventListener.handler(events.PersonNormalMessageReceived)` and related event classes
- `EventContext.reply(MessageChain([...]))`
- `MessageChain`, `Plain`, `Image`

## Local Test

```bash
python -m pytest tests
```

## LangBot Runtime Debug

```bash
lbp run
```

`lbp run` must connect to Plugin Runtime. If no runtime is active, this is expected to fail or wait for a connection; do not treat that as a business test failure.

In this workspace, `lbp run --stdio` stayed running until the 15-second command timeout and printed no import error. Unit tests and `lbp build` are the deterministic first-round checks.
