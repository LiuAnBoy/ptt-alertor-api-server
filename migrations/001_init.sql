-- PTT Alertor Database Schema
-- Normalized Design

-- Boards table
CREATE TABLE IF NOT EXISTS boards (
    name        VARCHAR(50) PRIMARY KEY,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- Articles table
CREATE TABLE IF NOT EXISTS articles (
    code               VARCHAR(30) PRIMARY KEY,
    id                 INTEGER NOT NULL,
    title              TEXT NOT NULL,
    link               TEXT,
    date               VARCHAR(20),
    author             VARCHAR(50),
    board_name         VARCHAR(50) NOT NULL REFERENCES boards(name) ON DELETE CASCADE,
    push_sum           INTEGER DEFAULT 0,
    last_push_datetime TIMESTAMP,
    created_at         TIMESTAMP DEFAULT NOW(),
    updated_at         TIMESTAMP DEFAULT NOW()
);

-- Comments table
CREATE TABLE IF NOT EXISTS comments (
    id            SERIAL PRIMARY KEY,
    article_code  VARCHAR(30) NOT NULL REFERENCES articles(code) ON DELETE CASCADE,
    tag           VARCHAR(10),
    user_id       VARCHAR(50),
    content       TEXT,
    datetime      TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_articles_board ON articles(board_name);
CREATE INDEX IF NOT EXISTS idx_articles_author ON articles(author);
CREATE INDEX IF NOT EXISTS idx_articles_id ON articles(id);
CREATE INDEX IF NOT EXISTS idx_comments_article ON comments(article_code);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to boards
DROP TRIGGER IF EXISTS boards_updated_at ON boards;
CREATE TRIGGER boards_updated_at
    BEFORE UPDATE ON boards
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to articles
DROP TRIGGER IF EXISTS articles_updated_at ON articles;
CREATE TRIGGER articles_updated_at
    BEFORE UPDATE ON articles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
