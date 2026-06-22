from __future__ import annotations

import json
from pathlib import Path

from final_review_sales.models.conversation import ConversationState


class StateStore:
    def __init__(self, path: str | Path = ".final_review_sales_state.json"):
        self.path = Path(path)
        self._states: dict[str, ConversationState] = {}
        self.load()

    def load(self) -> None:
        if not self.path.exists():
            self._states = {}
            return

        data = json.loads(self.path.read_text(encoding="utf-8"))
        self._states = {
            key: ConversationState.from_dict(value)
            for key, value in data.items()
        }

    def save(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        data = {key: state.to_dict() for key, state in self._states.items()}
        self.path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")

    def upsert(self, state: ConversationState) -> ConversationState:
        self._states[state.key()] = state
        self.save()
        return state

    def get(self, chat_platform: str, chat_user_id: str, chat_group_id: str | None = None) -> ConversationState | None:
        group = chat_group_id or "private"
        return self._states.get(f"{chat_platform}:{group}:{chat_user_id}")

    def list_waiting_payment(self) -> list[ConversationState]:
        return [state for state in self._states.values() if state.state == "waiting_payment"]
