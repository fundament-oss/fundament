-- DCIM task-management seed for local development.
--
-- ID convention (continues 026_0101-content.sql): 019dc<TT>0-0000-7000-8000-
-- 0000000000NN, with TT=e1 for tasks, e2 for task_steps and e3 for users; task
-- notes continue the c0 (note) sequence.

-- ── Users (task-management roster) ──────────────────────────────────────────
-- The assignee dropdown source for task-management, and the identities the
-- technician page resolves the logged-in user against.
--
-- external_ref is the DCIM JWT subject, i.e. how the identity provider names
-- this person; GetCurrentUser matches on it and hands back the internal id that
-- tasks are actually assigned to. dcim-authn-api derives that subject from dex:
--   sub          = base64url(proto{user_id=<dex userID>, conn_id="local"})  (dex)
--   external_ref = uuid.NewSHA1(6ba7b810-9dad-11d1-80b4-00c04fd430c8, sub)  (dcim-authn-api subjectUUID)
-- The refs below are those of the dex-dcim dev accounts (charts/fundament/
-- values-local.yaml → dexDcim.staticPasswords), so any of them can log in and
-- land on a populated technician page with no manual DB edits. If the dev
-- accounts change, recompute these refs — verify against what /userinfo returns.
INSERT INTO dcim.users (id, external_ref, name, email) VALUES
    ('019dce30-0000-7000-8000-000000000001', '6b06ad37-5db1-5b84-8bea-57b287be639d', 'Alice', 'alice@acme-corp.com'),
    ('019dce30-0000-7000-8000-000000000002', '8c33fc91-9eae-5467-a3c0-d9a1b29acdf4', 'Bart',  'bart@acme-corp.com'),
    ('019dce30-0000-7000-8000-000000000003', 'c1cbb965-2a8b-5794-bebc-b1827c4bc0dd', 'Cindy', 'cindy@acme-corp.com'),
    ('019dce30-0000-7000-8000-000000000004', 'd6a18af6-af2b-5272-83aa-e9990309a99d', 'David', 'david@globex.com'),
    ('019dce30-0000-7000-8000-000000000005', 'fe571fce-3faa-5d6f-82dc-218e710071f6', 'Emily', 'emily@globex.com');

-- ── Tasks (work orders) ─────────────────────────────────────────────────────
-- TT=e1. The three technician-flow tasks (01-03) are assigned to Alice so her
-- technician page renders end to end on first login; the rest spread across the
-- roster and cover every status/priority/category for the admin board.
INSERT INTO dcim.tasks (id, title, description, status, priority, category, assignee_id, due_date, location, created) VALUES
    ('019dce10-0000-7000-8000-000000000001', 'Replace broken harddisk',
        'Failed disk in Bay 3 of backup-srv-07 at Rack 123. Replace with Seagate Exos X18 (ST16000NM000J, 16 TB). The RAID controller shows the drive as failed since yesterday evening.',
        'in_progress', 'critical', 'hardware', '019dce30-0000-7000-8000-000000000001', '2026-03-20 00:00:00+00', 'DC Amsterdam-West · Rack 123', '2026-03-15 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000002', 'Replace network switch — Rack 87',
        'The Cisco Nexus switch in Rack 87 has intermittent port failures on ports 24-28. Replace with the new Arista unit from stock.',
        'in_progress', 'high', 'network', '019dce30-0000-7000-8000-000000000001', '2026-03-19 00:00:00+00', 'DC Amsterdam-West · Rack 87', '2026-03-14 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000003', 'Inspect PDU — Hall A',
        'Routine quarterly inspection of the PDU in Hall A. Check all breakers, verify load balancing, and ensure no burnt contacts.',
        'ready', 'medium', 'power', '019dce30-0000-7000-8000-000000000001', '2026-03-25 00:00:00+00', 'DC Amsterdam-West · Hall A', '2026-03-16 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000004', 'Check cooling unit — Row 5',
        'Temperature sensors in Row 5 are reporting 2°C above normal baseline. Inspect the cooling unit for potential blockage or fan failure.',
        'ready', 'high', 'cooling', NULL, '2026-03-21 00:00:00+00', 'DC Amsterdam-West · Hall A, Row 5', '2026-03-17 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000005', 'Firmware update — UPS units Hall B',
        'Apply firmware v4.2.1 to all three Eaton UPS units in Hall B. Requires sequential update — do not update all at once.',
        'review', 'medium', 'power', '019dce30-0000-7000-8000-000000000002', '2026-03-22 00:00:00+00', 'DC Amsterdam-West · Hall B', '2026-03-13 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000006', 'Install additional cameras — Entrance B',
        'Mount two new security cameras at Entrance B as per the security audit recommendations. Cabling is already in place.',
        'blocked', 'low', 'security', '019dce30-0000-7000-8000-000000000003', '2026-03-28 00:00:00+00', 'DC Amsterdam-West · Entrance B', '2026-03-10 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000007', 'Decommission server DB-14',
        'Server DB-14 in Rack 45 has been migrated to new hardware. Wipe disks, remove from rack, and update asset inventory.',
        'done', 'low', 'hardware', '019dce30-0000-7000-8000-000000000004', '2026-03-17 00:00:00+00', 'DC Amsterdam-West · Rack 45', '2026-03-08 09:00:00+00'),
    ('019dce10-0000-7000-8000-000000000008', 'Repair cable management — Rack 92',
        'Cables in Rack 92 are obstructing airflow. Re-route and zip-tie all patch cables. Replace any damaged cables.',
        'in_progress', 'medium', 'hardware', '019dce30-0000-7000-8000-000000000005', '2026-03-23 00:00:00+00', 'DC Amsterdam-West · Rack 92', '2026-03-16 09:00:00+00');

-- ── Task steps (technician-flow checklists) ─────────────────────────────────
-- TT=e2. Steps for the three tasks assigned to Alice (01-03).
INSERT INTO dcim.task_steps (id, task_id, title, description, ordinal) VALUES
    -- Replace broken harddisk
    ('019dce20-0000-7000-8000-000000000001', '019dce10-0000-7000-8000-000000000001', 'Navigate to data center Hall B',          'Head to Hall B via the main corridor. Follow the blue floor markers. Your destination is Row 12, approximately halfway down the hall on the left side.', 1),
    ('019dce20-0000-7000-8000-000000000002', '019dce10-0000-7000-8000-000000000001', 'Enter the cold aisle',                     'Use your access badge on the card reader to enter the cold aisle between Row 12 and Row 13. The door will lock behind you automatically.', 2),
    ('019dce20-0000-7000-8000-000000000003', '019dce10-0000-7000-8000-000000000001', 'Locate Rack 123',                          'Rack 123 is on the left side of the aisle, the 4th rack from the entrance. It has a label plate reading "R-123" at the top.', 3),
    ('019dce20-0000-7000-8000-000000000004', '019dce10-0000-7000-8000-000000000001', 'Open the rack',                            'Enter access code 4591 on the rack''s keypad lock. The lock indicator LED will turn green. Pull the handle to open the front door.', 4),
    ('019dce20-0000-7000-8000-000000000005', '019dce10-0000-7000-8000-000000000001', 'Locate device "backup-srv-07" at U32',     'Count rack units from the bottom. U32 is in the upper third of the rack. The server is a 2U unit with a dark gray bezel.', 5),
    ('019dce20-0000-7000-8000-000000000006', '019dce10-0000-7000-8000-000000000001', 'Remove failed harddisk (Bay 3, top-left)', 'Put on your anti-static wrist strap and ground yourself. Press the orange release latch on Bay 3 and slide the caddy out gently.', 6),
    ('019dce20-0000-7000-8000-000000000007', '019dce10-0000-7000-8000-000000000001', 'Install replacement harddisk',             'Align the new Seagate Exos X18 caddy with Bay 3 rails and slide it in firmly until it clicks into place.', 7),
    ('019dce20-0000-7000-8000-000000000008', '019dce10-0000-7000-8000-000000000001', 'Verify & close up',                        'Wait 30 seconds for the RAID controller to detect the new drive. The Bay 3 LED should be solid green. Close and lock the rack door.', 8),
    -- Replace network switch
    ('019dce20-0000-7000-8000-000000000009', '019dce10-0000-7000-8000-000000000002', 'Navigate to Rack 87',                      'Head to Row 9 in Hall B. Rack 87 is on the right side of the aisle, the 2nd rack from the entrance. The label plate reads "R-087".', 1),
    ('019dce20-0000-7000-8000-00000000000a', '019dce10-0000-7000-8000-000000000002', 'Open rack & locate switch at U18',         'Enter code 7823 on the keypad. U18 holds a 1U Cisco switch labeled "sw-core-03" with an amber status LED — this is the failed unit.', 2),
    ('019dce20-0000-7000-8000-00000000000b', '019dce10-0000-7000-8000-000000000002', 'Remove failed switch',                     'Label all connected cables with the provided tags before disconnecting. Unscrew the rack ears (2 screws each side) and slide the switch forward.', 3),
    ('019dce20-0000-7000-8000-00000000000c', '019dce10-0000-7000-8000-000000000002', 'Install Cisco Catalyst 9200L',             'Slide the new switch into U18. Secure with rack ear screws. Re-connect cables in the order matching your labels.', 4),
    ('019dce20-0000-7000-8000-00000000000d', '019dce10-0000-7000-8000-000000000002', 'Verify connectivity & close rack',         'Wait 2 minutes for the switch to boot. All port LEDs should turn green. Confirm "sw-core-03" is back online on the NOC dashboard.', 5),
    -- Inspect PDU
    ('019dce20-0000-7000-8000-00000000000e', '019dce10-0000-7000-8000-000000000003', 'Navigate to PDU — Hall A, Row 3',          'Head to Hall A via the main corridor. The PDU is a vertical unit mounted on the right side of Rack 42, Row 3, labeled "PDU-A-042".', 1),
    ('019dce20-0000-7000-8000-00000000000f', '019dce10-0000-7000-8000-000000000003', 'Record power load readings',               'Use the multimeter to measure input voltage on all three phases. Expected: 220-240V each. Note any circuit above 80% capacity.', 2),
    ('019dce20-0000-7000-8000-000000000010', '019dce10-0000-7000-8000-000000000003', 'Inspect cable management & outlets',       'Check for loose cables, damaged outlets, or signs of heat stress. Verify all outlet covers are in place on unused ports.', 3),
    ('019dce20-0000-7000-8000-000000000011', '019dce10-0000-7000-8000-000000000003', 'Document findings & close',                'Record all readings and observations. If any circuit is above 80% load or anomalies were found, flag the issue in the system.', 4);

-- ── Task-scoped notes ───────────────────────────────────────────────────────
-- Authors are roster users: created_by (free text) became created_by_id (an FK
-- to dcim.users) in migration 030. The note that used to read "Admin" has no
-- roster entry, and the asset/placement notes from 026_0101-content.sql predate
-- the user directory, so both keep a null author.
INSERT INTO dcim.notes (id, body, created_by_id, task_id) VALUES
    ('019dcc00-0000-7000-8000-000000000005', 'Arrived at rack. Disk bay 3 LED is solid red. Starting replacement procedure.',     '019dce30-0000-7000-8000-000000000001', '019dce10-0000-7000-8000-000000000001'),
    ('019dcc00-0000-7000-8000-000000000006', 'Spare disk is available in storage room B, shelf 3. Serial: ZLR1N5JY.',            NULL,                                   '019dce10-0000-7000-8000-000000000001'),
    ('019dcc00-0000-7000-8000-000000000007', 'Migration window confirmed with NOC for tonight 22:00-02:00. Pre-staging the replacement switch now.', '019dce30-0000-7000-8000-000000000001', '019dce10-0000-7000-8000-000000000002'),
    ('019dcc00-0000-7000-8000-000000000008', 'Cameras arrived but mounting brackets are the wrong model. Waiting for replacement brackets from supplier.', '019dce30-0000-7000-8000-000000000003', '019dce10-0000-7000-8000-000000000006');
