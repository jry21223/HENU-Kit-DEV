DELETE FROM role_permissions WHERE permission_code IN ('quizcraft.workshop.read', 'quizcraft.workshop.write', 'quizcraft.workshop.publish');
DELETE FROM permission_codes WHERE code IN ('quizcraft.workshop.read', 'quizcraft.workshop.write', 'quizcraft.workshop.publish');
