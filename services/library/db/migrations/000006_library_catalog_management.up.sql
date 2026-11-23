CREATE TABLE IF NOT EXISTS library_courses (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    department text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS library_materials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id uuid NOT NULL REFERENCES library_courses(id) ON DELETE RESTRICT,
    title text NOT NULL,
    material_type text NOT NULL CHECK (material_type IN ('handout', 'exam', 'slides', 'exercise', 'answer', 'note', 'textbook')),
    description text NOT NULL DEFAULT '',
    access_level text NOT NULL DEFAULT 'public_free' CHECK (access_level IN ('public_free', 'authenticated', 'restricted')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_review', 'published', 'archived')),
    source_note text NOT NULL DEFAULT '',
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by uuid,
    updated_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS library_materials_course_idx ON library_materials (course_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS library_materials_status_idx ON library_materials (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS library_corrections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    material_id uuid REFERENCES library_materials(id) ON DELETE SET NULL,
    title text NOT NULL,
    details text NOT NULL,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'resolved', 'dismissed')),
    resolution_note text NOT NULL DEFAULT '',
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS library_corrections_status_idx ON library_corrections (status, updated_at DESC);
