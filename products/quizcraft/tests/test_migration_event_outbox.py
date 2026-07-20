import inspect

import db_storage


def test_migration_event_schema_captures_every_legacy_write_table():
    schema_source = inspect.getsource(db_storage.init_schema)

    assert "pg_advisory_xact_lock" in schema_source
    assert "source_transaction_id" in schema_source
    assert "ON CONFLICT DO NOTHING" in schema_source
    assert "quizcraft_capture_migration_event" in schema_source
    assert "AFTER INSERT OR UPDATE OR DELETE" in schema_source
    for table_name in (
        "question_banks",
        "bank_questions",
        "feedbacks",
        "users",
        "user_stats",
    ):
        assert f'"{table_name}"' in schema_source
