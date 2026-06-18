namespace InspectGive;

/// <summary>
/// Glove def indexes (ByMykel/CSGO-API). Gloves have no buyable classname — WeaponPaints
/// applies them onto the pawn's EconGloves from a <c>wp_player_gloves</c> row, so we only
/// need to recognise the def index.
/// </summary>
public static class GloveNames
{
    private static readonly HashSet<uint> ByDefIndex = new()
    {
        4725, // Broken Fang Gloves
        5027, // Bloodhound Gloves
        5030, // Sport Gloves
        5031, // Driver Gloves
        5032, // Hand Wraps
        5033, // Moto Gloves
        5034, // Specialist Gloves
        5035, // Hydra Gloves
    };

    /// <summary>True if the def index is a glove (routes to wp_player_gloves).</summary>
    public static bool IsGlove(uint defIndex) => ByDefIndex.Contains(defIndex);
}
