INSERT INTO permission_codes(code, description, status) VALUES
    ('quizcraft.workshop.read', 'Read QuizCraft Workshop banks within granted QuizCraft Scope', 'active'),
    ('quizcraft.workshop.write', 'Create, import, edit, and validate QuizCraft bank versions within granted QuizCraft Scope', 'active'),
    ('quizcraft.workshop.publish', 'Publish, unpublish, and roll back QuizCraft banks within granted QuizCraft Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description=EXCLUDED.description, status='active';
