using System.Globalization;
using MySqlConnector;

namespace InspectGive;

/// <summary>
/// Writes decoded inspect items into WeaponPaints' tables. See docs/cs2-inspect-codes.md §5.
///
/// Paint/seed/wear/StatTrak always go to <c>wp_player_skins</c> (guns, knives and gloves
/// alike). Knives and gloves additionally need a routing row (<c>wp_player_knife</c> /
/// <c>wp_player_gloves</c>) or WeaponPaints never swaps the model. Agents go to
/// <c>wp_player_agents</c> instead.
/// </summary>
public sealed class DatabaseHandler
{
    private readonly string _connectionString;

    public DatabaseHandler(InspectGiveConfig cfg)
    {
        _connectionString = new MySqlConnectionStringBuilder
        {
            Server = cfg.DatabaseHost,
            Port = cfg.DatabasePort,
            UserID = cfg.DatabaseUser,
            Password = cfg.DatabasePassword,
            Database = cfg.DatabaseName,
            Pooling = true,
        }.ConnectionString;
    }

    private const string Upsert = """
        INSERT INTO wp_player_skins
            (steamid, weapon_team, weapon_defindex, weapon_paint_id,
             weapon_wear, weapon_seed, weapon_nametag,
             weapon_stattrak, weapon_stattrak_count,
             weapon_sticker_0, weapon_sticker_1, weapon_sticker_2,
             weapon_sticker_3, weapon_sticker_4, weapon_keychain)
        VALUES
            (@steamid, @team, @defindex, @paint,
             @wear, @seed, @nametag,
             @stattrak, @stcount,
             @s0, @s1, @s2, @s3, @s4, @keychain)
        ON DUPLICATE KEY UPDATE
            weapon_paint_id       = VALUES(weapon_paint_id),
            weapon_wear           = VALUES(weapon_wear),
            weapon_seed           = VALUES(weapon_seed),
            weapon_nametag        = VALUES(weapon_nametag),
            weapon_stattrak       = VALUES(weapon_stattrak),
            weapon_stattrak_count = VALUES(weapon_stattrak_count),
            weapon_sticker_0      = VALUES(weapon_sticker_0),
            weapon_sticker_1      = VALUES(weapon_sticker_1),
            weapon_sticker_2      = VALUES(weapon_sticker_2),
            weapon_sticker_3      = VALUES(weapon_sticker_3),
            weapon_sticker_4      = VALUES(weapon_sticker_4),
            weapon_keychain       = VALUES(weapon_keychain);
        """;

    // Routing rows that make WeaponPaints actually swap a knife / glove model.
    private const string UpsertKnife = """
        INSERT INTO wp_player_knife (steamid, weapon_team, knife)
        VALUES (@steamid, @team, @knife)
        ON DUPLICATE KEY UPDATE knife = VALUES(knife);
        """;

    private const string UpsertGlove = """
        INSERT INTO wp_player_gloves (steamid, weapon_team, weapon_defindex)
        VALUES (@steamid, @team, @defindex)
        ON DUPLICATE KEY UPDATE weapon_defindex = VALUES(weapon_defindex);
        """;

    // One row per player, CT/T columns. COALESCE keeps the other side's existing agent
    // when only one column is written (native-side-only).
    private const string UpsertAgent = """
        INSERT INTO wp_player_agents (steamid, agent_ct, agent_t)
        VALUES (@steamid, @ct, @t)
        ON DUPLICATE KEY UPDATE
            agent_ct = COALESCE(VALUES(agent_ct), agent_ct),
            agent_t  = COALESCE(VALUES(agent_t), agent_t);
        """;

    public async Task ApplyAsync(ulong steamId, InspectItem item, bool bothTeams)
    {
        await using var conn = new MySqlConnection(_connectionString);
        await conn.OpenAsync();

        // Knives need their classname for the wp_player_knife routing row.
        var knifeClass = KnifeNames.TryGet(item.DefIndex);
        var isGlove = GloveNames.IsGlove(item.DefIndex);

        // Stickers pack sequentially into columns 0...4
        var stickerCols = BuildStickerColumns(item.Stickers);
        var keychain = FormatKeychain(item.Keychain);

        // 2 = Terrorist, 3 = Counter-Terrorist.
        int[] teams = bothTeams ? new[] { 2, 3 } : new[] { 3 };
        foreach (var team in teams)
        {
            // Paint/seed/wear: needed for guns, knives and gloves.
            await using (var cmd = new MySqlCommand(Upsert, conn))
            {
                cmd.Parameters.AddWithValue("@steamid", steamId.ToString());
                cmd.Parameters.AddWithValue("@team", team);
                cmd.Parameters.AddWithValue("@defindex", item.DefIndex);
                cmd.Parameters.AddWithValue("@paint", item.PaintIndex);
                cmd.Parameters.AddWithValue("@wear", item.Wear);
                cmd.Parameters.AddWithValue("@seed", item.Seed);
                cmd.Parameters.AddWithValue("@nametag", item.NameTag);
                cmd.Parameters.AddWithValue("@stattrak", item.StatTrak ? 1 : 0);
                cmd.Parameters.AddWithValue("@stcount", item.StatTrakCount);
                cmd.Parameters.AddWithValue("@s0", stickerCols[0]);
                cmd.Parameters.AddWithValue("@s1", stickerCols[1]);
                cmd.Parameters.AddWithValue("@s2", stickerCols[2]);
                cmd.Parameters.AddWithValue("@s3", stickerCols[3]);
                cmd.Parameters.AddWithValue("@s4", stickerCols[4]);
                cmd.Parameters.AddWithValue("@keychain", keychain);
                await cmd.ExecuteNonQueryAsync();
            }

            if (knifeClass is not null)
            {
                await using var cmd = new MySqlCommand(UpsertKnife, conn);
                cmd.Parameters.AddWithValue("@steamid", steamId.ToString());
                cmd.Parameters.AddWithValue("@team", team);
                cmd.Parameters.AddWithValue("@knife", knifeClass);
                await cmd.ExecuteNonQueryAsync();
            }
            else if (isGlove)
            {
                await using var cmd = new MySqlCommand(UpsertGlove, conn);
                cmd.Parameters.AddWithValue("@steamid", steamId.ToString());
                cmd.Parameters.AddWithValue("@team", team);
                cmd.Parameters.AddWithValue("@defindex", item.DefIndex);
                await cmd.ExecuteNonQueryAsync();
            }
        }
    }

    /// <summary>Writes an agent to <c>wp_player_agents</c>. When <paramref name="bothSides"/>
    /// the slug goes to both columns; otherwise only the agent's native-faction column.</summary>
    public async Task ApplyAgentAsync(ulong steamId, AgentInfo agent, bool bothSides)
    {
        await using var conn = new MySqlConnection(_connectionString);
        await conn.OpenAsync();

        // Null leaves the column untouched (see COALESCE in UpsertAgent).
        string? ct = bothSides || agent.IsCounterTerrorist ? agent.Slug : null;
        string? t = bothSides || !agent.IsCounterTerrorist ? agent.Slug : null;

        await using var cmd = new MySqlCommand(UpsertAgent, conn);
        cmd.Parameters.AddWithValue("@steamid", steamId.ToString());
        cmd.Parameters.AddWithValue("@ct", (object?)ct ?? DBNull.Value);
        cmd.Parameters.AddWithValue("@t", (object?)t ?? DBNull.Value);
        await cmd.ExecuteNonQueryAsync();
    }

    /// <summary>Renders stickers into the five <c>weapon_sticker_N</c> strings, each
    /// <c>"id;schema;x;y;wear;scale;rotation"</c> (schema always 0). Extras beyond five are dropped.</summary>
    private static string[] BuildStickerColumns(IReadOnlyList<InspectSticker> stickers)
    {
        var cols = new[] { "", "", "", "", "" };
        for (var i = 0; i < cols.Length && i < stickers.Count; i++)
        {
            var s = stickers[i];
            cols[i] = string.Join(';',
                s.Id.ToString(CultureInfo.InvariantCulture),
                "0",
                F(s.OffsetX), F(s.OffsetY), F(s.Wear), F(s.Scale), F(s.Rotation));
        }
        return cols;
    }

    /// <summary>Renders the charm into <c>weapon_keychain</c> (<c>"id;x;y;z;seed"</c>), or ""
    /// when absent — WeaponPaints splits this column and would NRE on null.</summary>
    private static string FormatKeychain(InspectKeychain? kc)
    {
        if (kc is null)
            return "";
        return string.Join(';',
            kc.Id.ToString(CultureInfo.InvariantCulture),
            F(kc.OffsetX), F(kc.OffsetY), F(kc.OffsetZ),
            kc.Seed.ToString(CultureInfo.InvariantCulture));
    }

    private static string F(float v) => v.ToString(CultureInfo.InvariantCulture);
}
