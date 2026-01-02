-- InfraPilot Database Seed Script
-- Run with: psql -U infrapilot -d infrapilot -f scripts/seed.sql
-- Or: ./scripts/dev.sh seed

-- Create default organization
-- This is created automatically during setup, but useful for development
INSERT INTO organizations (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Organization', 'default')
ON CONFLICT (slug) DO NOTHING;

-- Note: No default users are seeded.
-- On first access, users will be prompted to create an admin account.
-- This is more secure than having default credentials.

-- Output confirmation
SELECT 'Seed data created successfully!' AS status;
SELECT 'On first access, you will be prompted to create an admin account.' AS info;
