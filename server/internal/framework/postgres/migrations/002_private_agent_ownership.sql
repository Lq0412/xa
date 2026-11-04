UPDATE agent_definitions
SET owner_type = 'user', owner_id = created_by
WHERE visibility = 'private' AND owner_type = 'system';
