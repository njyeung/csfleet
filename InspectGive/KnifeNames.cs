namespace InspectGive;

/// <summary>
/// Maps a knife/bayonet def index to its entity classname. The classname is written to
/// <c>wp_player_knife</c> to make WeaponPaints swap the model; the paint still comes from
/// the <c>wp_player_skins</c> row. Knives aren't given as items — the player always has one.
/// </summary>
public static class KnifeNames
{
    private static readonly Dictionary<uint, string> ByDefIndex = new()
    {
        [500] = "weapon_bayonet",
        [503] = "weapon_knife_css",             // Classic Knife
        [505] = "weapon_knife_flip",
        [506] = "weapon_knife_gut",
        [507] = "weapon_knife_karambit",
        [508] = "weapon_knife_m9_bayonet",
        [509] = "weapon_knife_tactical",        // Huntsman
        [512] = "weapon_knife_falchion",
        [514] = "weapon_knife_survival_bowie",  // Bowie
        [515] = "weapon_knife_butterfly",
        [516] = "weapon_knife_push",            // Shadow Daggers
        [517] = "weapon_knife_cord",            // Paracord
        [518] = "weapon_knife_canis",           // Survival Knife
        [519] = "weapon_knife_ursus",
        [520] = "weapon_knife_gypsy_jackknife", // Navaja
        [521] = "weapon_knife_outdoor",         // Nomad
        [522] = "weapon_knife_stiletto",
        [523] = "weapon_knife_widowmaker",      // Talon
        [525] = "weapon_knife_skeleton",
        [526] = "weapon_knife_kukri",
    };

    /// <summary>Knife entity classname for a def index, or null if not a known knife.</summary>
    public static string? TryGet(uint defIndex) =>
        ByDefIndex.TryGetValue(defIndex, out var name) ? name : null;

    /// <summary>True if the def index is a knife/bayonet (routes to wp_player_knife).</summary>
    public static bool IsKnife(uint defIndex) => ByDefIndex.ContainsKey(defIndex);
}
