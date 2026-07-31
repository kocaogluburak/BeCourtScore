-- ============================================================================
-- CourtScore — "Test 3" turnuvasına 16 katılımcı ekler
--
-- Turnuva: cde4138f-b095-4ca7-b5a6-4d8ecb675108  (Test 3, PADEL, SINGLE_ELIM)
-- Durum   : REGISTRATION olarak KALIR — draw/lock YAPILMAZ, maç üretilmez.
--           Bu yüzden bu script "kazanan" oluşturmaz; turnuva hiç başlatılmamış
--           gibi, sadece 16 kişi kayıtlı görünür.
--
-- Katılımcılar (16):
--   * 878be389-… Burak Kocaoglu   (turnuva organizatörü)
--   * 39ccb922-… Burkii
--   * 00000000-…-0001 … 0014      (ilk 14 test kullanıcısı)
--
-- Şema notları:
--   * display_name, addParticipant ile aynı mantıkla snapshot'lanır:
--     nickname → yoksa "name surname" → yoksa 'Player'.
--   * status DEFAULT 'CONFIRMED', seed NULL kalır (henüz draw yok).
--   * UNIQUE(tournament_id, user_id) sayesinde tekrar çalıştırılabilir.
--
-- Çalıştırma (Docker Postgres):
--   docker exec -i courtscore_db psql -U courtscore -d courtscore \
--     < scripts/seed_test3_participants.sql
--
-- Ya da psql ile:
--   psql "postgres://courtscore:courtscore@localhost:5432/courtscore?sslmode=disable" \
--     -f scripts/seed_test3_participants.sql
--
-- Sadece LOCAL geliştirme içindir. Prod DB'de ÇALIŞTIRMAYIN.
-- ============================================================================

INSERT INTO tournament_participants (tournament_id, user_id, display_name)
SELECT
    'cde4138f-b095-4ca7-b5a6-4d8ecb675108',
    u.id,
    COALESCE(
        NULLIF(u.nickname, ''),
        NULLIF(TRIM(CONCAT_WS(' ', u.name, u.surname)), ''),
        'Player'
    )
FROM users u
WHERE u.id IN (
    '878be389-bc54-4d04-94f2-c8f9865877e8',  -- Burak Kocaoglu (organizatör)
    '39ccb922-c3c6-484d-afcb-396508e0c857',  -- Burkii
    '00000000-0000-4000-a000-000000000001',  -- ace_ahmet
    '00000000-0000-4000-a000-000000000002',  -- baseline_burak
    '00000000-0000-4000-a000-000000000003',  -- volkan_volley
    '00000000-0000-4000-a000-000000000004',  -- smash_selin
    '00000000-0000-4000-a000-000000000005',  -- dropshot_deniz
    '00000000-0000-4000-a000-000000000006',  -- topspin_tolga
    '00000000-0000-4000-a000-000000000007',  -- lob_leyla
    '00000000-0000-4000-a000-000000000008',  -- rally_rana
    '00000000-0000-4000-a000-000000000009',  -- serve_sinan
    '00000000-0000-4000-a000-000000000010',  -- net_nazli
    '00000000-0000-4000-a000-000000000011',  -- court_cem
    '00000000-0000-4000-a000-000000000012',  -- match_mert
    '00000000-0000-4000-a000-000000000013',  -- deuce_defne
    '00000000-0000-4000-a000-000000000014'   -- slice_sena
)
ON CONFLICT (tournament_id, user_id) DO NOTHING;

-- Kontrol: kaç kişi kayıtlı ve turnuva hâlâ REGISTRATION mı?
--   SELECT
--       (SELECT status FROM tournaments WHERE id = 'cde4138f-b095-4ca7-b5a6-4d8ecb675108') AS tournament_status,
--       COUNT(*) AS confirmed_participants
--   FROM tournament_participants
--   WHERE tournament_id = 'cde4138f-b095-4ca7-b5a6-4d8ecb675108'
--     AND status = 'CONFIRMED';
--
--   SELECT tp.display_name, tp.status, u.email
--   FROM tournament_participants tp
--   JOIN users u ON u.id = tp.user_id
--   WHERE tp.tournament_id = 'cde4138f-b095-4ca7-b5a6-4d8ecb675108'
--   ORDER BY tp.created_at;
