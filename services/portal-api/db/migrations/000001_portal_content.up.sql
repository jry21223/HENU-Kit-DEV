-- Portal API content tables (MySQL)
-- These tables store user-facing content that doesn't exist in other services.

-- Food posts (consumer-facing food reviews)
CREATE TABLE IF NOT EXISTS portal_food_posts (
    id VARCHAR(64) PRIMARY KEY,
    campus ENUM('minglun', 'jinming', 'longzihu') NOT NULL,
    title VARCHAR(255) NOT NULL,
    excerpt TEXT NOT NULL,
    blocks JSON NOT NULL,
    author VARCHAR(128) NOT NULL,
    likes INT NOT NULL DEFAULT 0,
    stars INT NOT NULL DEFAULT 0,
    tags JSON NOT NULL,
    shop_name VARCHAR(255) NOT NULL DEFAULT '',
    shop_lat DOUBLE NOT NULL DEFAULT 0,
    shop_lng DOUBLE NOT NULL DEFAULT 0,
    `time` VARCHAR(32) NOT NULL,
    hidden TINYINT(1) NOT NULL DEFAULT 0,
    images JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_campus (campus),
    INDEX idx_hidden (hidden)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Food comments
CREATE TABLE IF NOT EXISTS portal_food_comments (
    id VARCHAR(64) PRIMARY KEY,
    post_id VARCHAR(64) NOT NULL,
    author VARCHAR(128) NOT NULL,
    `time` VARCHAR(32) NOT NULL,
    `text` TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_post (post_id),
    FOREIGN KEY (post_id) REFERENCES portal_food_posts(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Campus marketplace items
CREATE TABLE IF NOT EXISTS portal_campus_items (
    id VARCHAR(64) PRIMARY KEY,
    type ENUM('help', 'sell') NOT NULL,
    category VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    `desc` TEXT NOT NULL,
    price DOUBLE NOT NULL DEFAULT 0,
    seller VARCHAR(128) NOT NULL,
    credit INT NOT NULL DEFAULT 80,
    deals_done INT NOT NULL DEFAULT 0,
    wants INT NOT NULL DEFAULT 0,
    place VARCHAR(255) NOT NULL DEFAULT '',
    deadline VARCHAR(64) DEFAULT NULL,
    status ENUM('open', 'ongoing', 'done', 'hidden') NOT NULL DEFAULT 'open',
    `time` VARCHAR(32) NOT NULL,
    images JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_type (type),
    INDEX idx_status (status),
    INDEX idx_category (category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Campus item messages
CREATE TABLE IF NOT EXISTS portal_campus_messages (
    id VARCHAR(64) PRIMARY KEY,
    item_id VARCHAR(64) NOT NULL,
    author VARCHAR(128) NOT NULL,
    `time` VARCHAR(32) NOT NULL,
    `text` TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_item (item_id),
    FOREIGN KEY (item_id) REFERENCES portal_campus_items(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
