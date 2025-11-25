-- Rename customer_id to subscriber_id
-- This better reflects ISP terminology where customers are "subscribers"

ALTER TABLE targets RENAME COLUMN customer_id TO subscriber_id;

-- Rename the index as well
ALTER INDEX idx_targets_customer RENAME TO idx_targets_subscriber;
