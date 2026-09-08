DROP INDEX idx_announcements_feed;
ALTER TABLE announcements DROP COLUMN pinned_until, DROP COLUMN published_at;
CREATE INDEX idx_announcements_feed
    ON announcements (domain_id, pinned DESC, created_at DESC) WHERE published;
