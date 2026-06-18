namespace InspectGive;

/// <summary>
/// Maps a buyable gun's def index to its entity classname (e.g. <c>7 → "weapon_ak47"</c>),
/// used to hand the weapon over with <c>GiveNamedItem</c>. Knives and gloves live in
/// <see cref="KnifeNames"/> / <see cref="GloveNames"/>; agents in <see cref="AgentNames"/>.
/// </summary>
public static class WeaponNames
{
    private static readonly Dictionary<uint, string> ByDefIndex = new()
    {
        [1] = "weapon_deagle",
        [2] = "weapon_elite",
        [3] = "weapon_fiveseven",
        [4] = "weapon_glock",
        [7] = "weapon_ak47",
        [8] = "weapon_aug",
        [9] = "weapon_awp",
        [10] = "weapon_famas",
        [11] = "weapon_g3sg1",
        [13] = "weapon_galilar",
        [14] = "weapon_m249",
        [16] = "weapon_m4a1",           // M4A4
        [17] = "weapon_mac10",
        [19] = "weapon_p90",
        [23] = "weapon_mp5sd",
        [24] = "weapon_ump45",
        [25] = "weapon_xm1014",
        [26] = "weapon_bizon",
        [27] = "weapon_mag7",
        [28] = "weapon_negev",
        [29] = "weapon_sawedoff",
        [30] = "weapon_tec9",
        [31] = "weapon_taser",
        [32] = "weapon_hkp2000",        // P2000
        [33] = "weapon_mp7",
        [34] = "weapon_mp9",
        [35] = "weapon_nova",
        [36] = "weapon_p250",
        [38] = "weapon_scar20",
        [39] = "weapon_sg556",          // SG 553
        [40] = "weapon_ssg08",
        [60] = "weapon_m4a1_silencer",  // M4A1-S
        [61] = "weapon_usp_silencer",   // USP-S
        [63] = "weapon_cz75a",
        [64] = "weapon_revolver",       // R8
    };

    /// <summary>Entity classname for a gun def index, or null if not a known gun.</summary>
    public static string? TryGet(uint defIndex) =>
        ByDefIndex.TryGetValue(defIndex, out var name) ? name : null;

    /// <summary>True if the def index is a buyable gun (routes to wp_player_skins, can be given).</summary>
    public static bool IsWeapon(uint defIndex) => ByDefIndex.ContainsKey(defIndex);
}
