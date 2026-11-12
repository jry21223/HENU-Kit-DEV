-- Core-owned fixed backup interface. Only anonymous row digests leave PostgreSQL.
SELECT relation,row_sha256 FROM (
SELECT '@catalog'::text AS relation,''::text AS row_sha256
UNION ALL
SELECT '@catalog',encode(sha256(convert_to(record,'UTF8')),'hex') FROM (WITH objects(kind,name,definition) AS (
 SELECT 'relation',c.relname,jsonb_build_array(c.relkind,c.relpersistence,c.relrowsecurity,c.relforcerowsecurity,c.relreplident,c.reloptions)::text
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname LIKE 'quizcraft\_%' ESCAPE '\'
 UNION ALL SELECT 'column',c.relname||'.'||a.attnum,jsonb_build_array(a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,a.attidentity,a.attgenerated,coalesce(pg_get_expr(d.adbin,d.adrelid,false),''))::text
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
 WHERE n.nspname='public' AND c.relname LIKE 'quizcraft\_%' ESCAPE '\' AND a.attnum>0 AND NOT a.attisdropped
 UNION ALL SELECT 'constraint',c.relname||'.'||k.conname,jsonb_build_array(k.contype,k.condeferrable,k.condeferred,k.convalidated,pg_get_constraintdef(k.oid,false))::text
 FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname LIKE 'quizcraft\_%' ESCAPE '\'
 UNION ALL SELECT 'index',i.relname,jsonb_build_array(x.indisvalid,x.indisready,x.indisunique,x.indisprimary,pg_get_indexdef(i.oid,0,false))::text
 FROM pg_index x JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_namespace n ON n.oid=i.relnamespace WHERE n.nspname='public' AND i.relname LIKE 'quizcraft\_%' ESCAPE '\'
 UNION ALL SELECT 'trigger',c.relname||'.'||t.tgname,jsonb_build_array(t.tgenabled,t.tgtype,pg_get_triggerdef(t.oid,false))::text
 FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname LIKE 'quizcraft\_%' ESCAPE '\' AND NOT t.tgisinternal
 UNION ALL SELECT 'function',p.oid::regprocedure::text,pg_get_functiondef(p.oid)
 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname LIKE 'quizcraft\_%' ESCAPE '\'
 UNION ALL SELECT 'view',c.relname,pg_get_viewdef(c.oid,false) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname LIKE 'quizcraft\_%' ESCAPE '\' AND c.relkind='v'
 UNION ALL SELECT 'sequence',c.relname,jsonb_build_array(s.seqstart,s.seqincrement,s.seqmax,s.seqmin,s.seqcache,s.seqcycle)::text
 FROM pg_sequence s JOIN pg_class c ON c.oid=s.seqrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname LIKE 'quizcraft\_%' ESCAPE '\'
) SELECT jsonb_build_array(kind,name,definition)::text FROM objects ORDER BY kind COLLATE "C",name COLLATE "C",definition COLLATE "C") AS catalog(record)
UNION ALL
SELECT 'quizcraft_bank_version_questions',''
UNION ALL
SELECT 'quizcraft_bank_version_questions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_bank_version_questions AS source_row
UNION ALL
SELECT 'quizcraft_bank_versions',''
UNION ALL
SELECT 'quizcraft_bank_versions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_bank_versions AS source_row
UNION ALL
SELECT 'quizcraft_banks',''
UNION ALL
SELECT 'quizcraft_banks',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_banks AS source_row
UNION ALL
SELECT 'quizcraft_favorites',''
UNION ALL
SELECT 'quizcraft_favorites',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_favorites AS source_row
UNION ALL
SELECT 'quizcraft_feedback_inbox_deliveries',''
UNION ALL
SELECT 'quizcraft_feedback_inbox_deliveries',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_feedback_inbox_deliveries AS source_row
UNION ALL
SELECT 'quizcraft_feedback_inbox_outbox',''
UNION ALL
SELECT 'quizcraft_feedback_inbox_outbox',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_feedback_inbox_outbox AS source_row
UNION ALL
SELECT 'quizcraft_feedback_status_facts',''
UNION ALL
SELECT 'quizcraft_feedback_status_facts',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_feedback_status_facts AS source_row
UNION ALL
SELECT 'quizcraft_feedbacks',''
UNION ALL
SELECT 'quizcraft_feedbacks',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_feedbacks AS source_row
UNION ALL
SELECT 'quizcraft_idempotency_results',''
UNION ALL
SELECT 'quizcraft_idempotency_results',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_idempotency_results AS source_row
UNION ALL
SELECT 'quizcraft_learning_state',''
UNION ALL
SELECT 'quizcraft_learning_state',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_learning_state AS source_row
UNION ALL
SELECT 'quizcraft_legacy_feedback_archives',''
UNION ALL
SELECT 'quizcraft_legacy_feedback_archives',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_legacy_feedback_archives AS source_row
UNION ALL
SELECT 'quizcraft_legacy_feedback_state_events',''
UNION ALL
SELECT 'quizcraft_legacy_feedback_state_events',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_legacy_feedback_state_events AS source_row
UNION ALL
SELECT 'quizcraft_legacy_ranking_snapshots',''
UNION ALL
SELECT 'quizcraft_legacy_ranking_snapshots',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_legacy_ranking_snapshots AS source_row
UNION ALL
SELECT 'quizcraft_migration_event_receipts',''
UNION ALL
SELECT 'quizcraft_migration_event_receipts',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_migration_event_receipts AS source_row
UNION ALL
SELECT 'quizcraft_migration_exception_resolutions',''
UNION ALL
SELECT 'quizcraft_migration_exception_resolutions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_migration_exception_resolutions AS source_row
UNION ALL
SELECT 'quizcraft_migration_exceptions',''
UNION ALL
SELECT 'quizcraft_migration_exceptions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_migration_exceptions AS source_row
UNION ALL
SELECT 'quizcraft_migration_runs',''
UNION ALL
SELECT 'quizcraft_migration_runs',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_migration_runs AS source_row
UNION ALL
SELECT 'quizcraft_practice_attempts',''
UNION ALL
SELECT 'quizcraft_practice_attempts',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_practice_attempts AS source_row
UNION ALL
SELECT 'quizcraft_practice_session_claims',''
UNION ALL
SELECT 'quizcraft_practice_session_claims',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_practice_session_claims AS source_row
UNION ALL
SELECT 'quizcraft_practice_session_questions',''
UNION ALL
SELECT 'quizcraft_practice_session_questions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_practice_session_questions AS source_row
UNION ALL
SELECT 'quizcraft_practice_sessions',''
UNION ALL
SELECT 'quizcraft_practice_sessions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_practice_sessions AS source_row
UNION ALL
SELECT 'quizcraft_question_stats',''
UNION ALL
SELECT 'quizcraft_question_stats',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_question_stats AS source_row
UNION ALL
SELECT 'quizcraft_question_versions',''
UNION ALL
SELECT 'quizcraft_question_versions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_question_versions AS source_row
UNION ALL
SELECT 'quizcraft_questions',''
UNION ALL
SELECT 'quizcraft_questions',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_questions AS source_row
UNION ALL
SELECT 'quizcraft_ranking_profiles',''
UNION ALL
SELECT 'quizcraft_ranking_profiles',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_ranking_profiles AS source_row
UNION ALL
SELECT 'quizcraft_ranking_settlement_events',''
UNION ALL
SELECT 'quizcraft_ranking_settlement_events',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_ranking_settlement_events AS source_row
UNION ALL
SELECT 'quizcraft_schema_migrations',''
UNION ALL
SELECT 'quizcraft_schema_migrations',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_schema_migrations AS source_row
UNION ALL
SELECT 'quizcraft_service_nonces',''
UNION ALL
SELECT 'quizcraft_service_nonces',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_service_nonces AS source_row
UNION ALL
SELECT 'quizcraft_shadow_comparisons',''
UNION ALL
SELECT 'quizcraft_shadow_comparisons',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_shadow_comparisons AS source_row
UNION ALL
SELECT 'quizcraft_shadow_gate_reports',''
UNION ALL
SELECT 'quizcraft_shadow_gate_reports',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_shadow_gate_reports AS source_row
UNION ALL
SELECT 'quizcraft_workshop_audit_events',''
UNION ALL
SELECT 'quizcraft_workshop_audit_events',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_workshop_audit_events AS source_row
UNION ALL
SELECT 'quizcraft_workshop_version_states',''
UNION ALL
SELECT 'quizcraft_workshop_version_states',encode(sha256(convert_to(to_jsonb(source_row)::text,'UTF8')),'hex') FROM public.quizcraft_workshop_version_states AS source_row
) AS evidence ORDER BY relation COLLATE "C",row_sha256 COLLATE "C"
