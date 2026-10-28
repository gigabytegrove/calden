-- Do not label current-status-only observations as physical carrier scans.
-- Preserve the entire prior checkpoint history and all deduplication refs.
ALTER TABLE package_checkpoints DROP CONSTRAINT IF EXISTS package_checkpoints_source_check;
ALTER TABLE package_checkpoints ADD CONSTRAINT package_checkpoints_source_check
 CHECK (source IN ('manual','email','provider','provider_status'));
UPDATE package_checkpoints SET source='provider_status'
 WHERE source='provider' AND (provider_ref LIKE 'local:current:%'
  OR provider_ref LIKE 'public:current:%'
  OR provider_ref LIKE 'aftership:current:%');
