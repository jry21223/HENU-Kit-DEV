WITH objects(kind,name,definition) AS (
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
) SELECT jsonb_build_array(kind,name,definition)::text FROM objects ORDER BY kind COLLATE "C",name COLLATE "C",definition COLLATE "C"
