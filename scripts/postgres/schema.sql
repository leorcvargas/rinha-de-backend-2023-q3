CREATE EXTENSION IF NOT EXISTS "unaccent";

CREATE EXTENSION IF NOT EXISTS "pg_trgm";

CREATE TABLE
    IF NOT EXISTS public.people (
        nickname varchar(32) PRIMARY KEY NOT NULL,
        id uuid NOT NULL,
        "name" varchar(100) NOT NULL,
        birthdate date NOT NULL,
        stack text NULL,
        search text NOT NULL
    );

-- ALTER TABLE public.people

-- ADD

--     COLUMN trgm_q text GENERATED ALWAYS AS (

--         nickname || ' ' || "name" || ' ' || stack

--     ) STORED;

CREATE INDEX
    CONCURRENTLY IF NOT EXISTS idx_people_trigram ON public.people USING gist (search gist_trgm_ops);