using System.Reflection;
using System.Text.Json;

namespace InspectGive;

/// <summary>
/// Turns a decoded <see cref="InspectItem"/> into a human-readable name like
/// <c>StatTrak™ AK-47 | Redline (Field-Tested)</c>, for chat feedback.
///
/// Backed by the embedded <c>SkinNames.json</c> ({"defindex_paintindex": "name"}),
/// refreshed from ByMykel/CSGO-API by plugin/build.sh on each build.
/// </summary>
public static class SkinNames
{
    private static readonly Dictionary<string, string> ByKey = Load();

    private static Dictionary<string, string> Load()
    {
        var asm = Assembly.GetExecutingAssembly();
        var name = asm.GetManifestResourceNames().FirstOrDefault(n => n.EndsWith("SkinNames.json"));
        if (name is null)
            return new();
        using var stream = asm.GetManifestResourceStream(name)!;
        using var reader = new StreamReader(stream);
        return JsonSerializer.Deserialize<Dictionary<string, string>>(reader.ReadToEnd()) ?? new();
    }

    /// <summary>Friendly name for the item, e.g. <c>StatTrak™ AWP | Asiimov (Well-Worn)</c>.
    /// Falls back to the weapon/knife classname (or "Unknown item") when the paint is
    /// unrecognised — a vanilla weapon, an obscure paint, or stale lookup data.</summary>
    public static string Describe(InspectItem item)
    {
        var stattrak = item.StatTrak ? "StatTrak™ " : "";

        if (ByKey.TryGetValue($"{item.DefIndex}_{item.PaintIndex}", out var name))
            return $"{stattrak}{name} ({Wear(item.Wear)})";

        var fallback = WeaponNames.TryGet(item.DefIndex)
            ?? KnifeNames.TryGet(item.DefIndex)
            ?? "Unknown item";
        return $"{stattrak}{fallback}";
    }

    // CS2 wear buckets (exterior the float maps to).
    private static string Wear(float f) => f switch
    {
        < 0.07f => "Factory New",
        < 0.15f => "Minimal Wear",
        < 0.38f => "Field-Tested",
        < 0.45f => "Well-Worn",
        _ => "Battle-Scarred",
    };
}
