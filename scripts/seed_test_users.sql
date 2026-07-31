-- ============================================================================
-- CourtScore — Local test kullanıcıları (20 adet)
--
-- Sadece LOCAL geliştirme içindir. Prod DB'de ÇALIŞTIRMAYIN.
--
-- Şema notları:
--   * users tablosunda parola yoktur; kimlik doğrulama auth_identities
--     üzerinden (provider = 'google') yapılır. Bu seed kullanıcıları
--     gerçek Google token'ı olmadığı için uygulamadan login OLAMAZ; amaçları
--     arama / arkadaşlık / turnuva gibi akışları test etmektir.
--   * profile_icon UI'da emoji olarak render edilir.
--   * Sabit UUID'ler kullanıldığı için script tekrar tekrar çalıştırılabilir
--     (ON CONFLICT DO NOTHING).
--
-- Çalıştırma (Docker Postgres):
--   docker exec -i courtscore_db psql -U courtscore -d courtscore \
--     < scripts/seed_test_users.sql
--
-- Ya da psql ile:
--   psql "postgres://courtscore:courtscore@localhost:5432/courtscore?sslmode=disable" \
--     -f scripts/seed_test_users.sql
-- ============================================================================

INSERT INTO users (id, email, email_verified, nickname, name, surname, profile_icon)
VALUES
    ('00000000-0000-4000-a000-000000000001', 'ace_ahmet@test.courtscore.local',      TRUE, 'ace_ahmet',      'Ahmet',  'Yılmaz',    '🎾'),
    ('00000000-0000-4000-a000-000000000002', 'baseline_burak@test.courtscore.local', TRUE, 'baseline_burak', 'Burak',  'Demir',     '🏓'),
    ('00000000-0000-4000-a000-000000000003', 'volkan_volley@test.courtscore.local',  TRUE, 'volkan_volley',  'Volkan', 'Kaya',      '🏸'),
    ('00000000-0000-4000-a000-000000000004', 'smash_selin@test.courtscore.local',    TRUE, 'smash_selin',    'Selin',  'Çelik',     '🥎'),
    ('00000000-0000-4000-a000-000000000005', 'dropshot_deniz@test.courtscore.local', TRUE, 'dropshot_deniz', 'Deniz',  'Arslan',    '🏐'),
    ('00000000-0000-4000-a000-000000000006', 'topspin_tolga@test.courtscore.local',  TRUE, 'topspin_tolga',  'Tolga',  'Şahin',     '⚡'),
    ('00000000-0000-4000-a000-000000000007', 'lob_leyla@test.courtscore.local',      TRUE, 'lob_leyla',      'Leyla',  'Aydın',     '🌟'),
    ('00000000-0000-4000-a000-000000000008', 'rally_rana@test.courtscore.local',     TRUE, 'rally_rana',     'Rana',   'Koç',       '🔥'),
    ('00000000-0000-4000-a000-000000000009', 'serve_sinan@test.courtscore.local',    TRUE, 'serve_sinan',    'Sinan',  'Yıldız',    '🚀'),
    ('00000000-0000-4000-a000-000000000010', 'net_nazli@test.courtscore.local',      TRUE, 'net_nazli',      'Nazlı',  'Öztürk',    '🎯'),
    ('00000000-0000-4000-a000-000000000011', 'court_cem@test.courtscore.local',      TRUE, 'court_cem',      'Cem',    'Aksoy',     '🏆'),
    ('00000000-0000-4000-a000-000000000012', 'match_mert@test.courtscore.local',     TRUE, 'match_mert',     'Mert',   'Doğan',     '💪'),
    ('00000000-0000-4000-a000-000000000013', 'deuce_defne@test.courtscore.local',    TRUE, 'deuce_defne',    'Defne',  'Kurt',      '🌈'),
    ('00000000-0000-4000-a000-000000000014', 'slice_sena@test.courtscore.local',     TRUE, 'slice_sena',     'Sena',   'Polat',     '🍀'),
    ('00000000-0000-4000-a000-000000000015', 'backhand_baran@test.courtscore.local', TRUE, 'backhand_baran', 'Baran',  'Erdoğan',   '🐯'),
    ('00000000-0000-4000-a000-000000000016', 'forehand_ferit@test.courtscore.local', TRUE, 'forehand_ferit', 'Ferit',  'Aslan',     '🦅'),
    ('00000000-0000-4000-a000-000000000017', 'spin_selim@test.courtscore.local',     TRUE, 'spin_selim',     'Selim',  'Taş',       '🐺'),
    ('00000000-0000-4000-a000-000000000018', 'game_gizem@test.courtscore.local',     TRUE, 'game_gizem',     'Gizem',  'Bulut',     '🦊'),
    ('00000000-0000-4000-a000-000000000019', 'set_serkan@test.courtscore.local',     TRUE, 'set_serkan',     'Serkan', 'Yavuz',     '🐉'),
    ('00000000-0000-4000-a000-000000000020', 'champ_ceren@test.courtscore.local',    TRUE, 'champ_ceren',    'Ceren',  'Acar',      '👑')
ON CONFLICT (id) DO NOTHING;

-- ── (Opsiyonel) auth_identities kayıtları ───────────────────────────────────
-- Referans bütünlüğü için "google" provider altında sahte subject'lerle
-- kimlik satırları oluşturur. Gerçek login SAĞLAMAZ (Google token doğrulaması
-- gerekir); yalnızca veriyi tam tutmak isterseniz açın.
--
-- INSERT INTO auth_identities (user_id, provider, provider_subject)
-- SELECT id, 'google', 'seed-' || id::text
-- FROM users
-- WHERE email LIKE '%@test.courtscore.local'
-- ON CONFLICT (provider, provider_subject) DO NOTHING;

-- Kontrol:
--   SELECT nickname, name, surname, email, profile_icon FROM users
--   WHERE email LIKE '%@test.courtscore.local' ORDER BY nickname;
