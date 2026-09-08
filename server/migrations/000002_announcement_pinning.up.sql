ALTER TABLE announcements
    ADD COLUMN pinned_until TIMESTAMPTZ,
    ADD COLUMN published_at TIMESTAMPTZ;

-- Existing published notices keep their original position in the timeline.
UPDATE announcements SET published_at = created_at WHERE published;

DROP INDEX idx_announcements_feed;
CREATE INDEX idx_announcements_feed
    ON announcements (domain_id, published_at DESC, id DESC) WHERE published;
