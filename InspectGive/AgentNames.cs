namespace InspectGive;

/// <summary>An agent (player model) identified by an inspect blob's def index.</summary>
/// <param name="Slug">The model path as WeaponPaints stores it in
/// <c>wp_player_agents.agent_ct/agent_t</c> — the middle slug only, e.g.
/// <c>ctm_sas/ctm_sas_variantf</c>. WeaponPaints applies it as
/// <c>SetModel($"agents/models/{slug}.vmdl")</c>, so the stored value has neither the
/// <c>agents/models/</c> prefix nor the <c>.vmdl</c> suffix.</param>
/// <param name="IsCounterTerrorist">The agent's native faction. Only used to pick the
/// column when not applying to both sides; the engine renders either model on either team.</param>
/// <param name="Name">Display name for chat feedback.</param>
public readonly record struct AgentInfo(string Slug, bool IsCounterTerrorist, string Name);

/// <summary>
/// Maps an agent's economy def index (decoded from an inspect blob) to its model slug,
/// faction and display name. The agent counterpart of <see cref="WeaponNames"/>.
///
/// Unlike weapons, agents are not written to <c>wp_player_skins</c> — they go to
/// WeaponPaints' separate <c>wp_player_agents</c> table (CT/T columns), and there's no
/// item to "give": WeaponPaints' <c>wp_refresh</c> swaps the player model on apply.
///
/// Source: ByMykel/CSGO-API <c>agents.json</c> (def_index, model_player, team). Slugs are
/// derived by stripping the <c>agents/models/</c> prefix and <c>.vmdl</c> suffix from
/// <c>model_player</c> to match WeaponPaints' stored format.
/// </summary>
public static class AgentNames
{
    private static readonly Dictionary<uint, AgentInfo> ByDefIndex = new()
    {
        [4613] = new("tm_professional/tm_professional_varf5", false, "Bloody Darryl The Strapped | The Professionals"),
        [4619] = new("ctm_st6/ctm_st6_variantj", true, "'Blueberries' Buckshot | NSWC SEAL"),
        [4680] = new("ctm_st6/ctm_st6_variantl", true, "'Two Times' McCoy | TACP Cavalry"),
        [4711] = new("ctm_swat/ctm_swat_variante", true, "Cmdr. Mae 'Dead Cold' Jamison | SWAT"),
        [4712] = new("ctm_swat/ctm_swat_variantf", true, "1st Lieutenant Farlow | SWAT"),
        [4713] = new("ctm_swat/ctm_swat_variantg", true, "John 'Van Healen' Kask | SWAT"),
        [4714] = new("ctm_swat/ctm_swat_varianth", true, "Bio-Haz Specialist | SWAT"),
        [4715] = new("ctm_swat/ctm_swat_varianti", true, "Sergeant Bombson | SWAT"),
        [4716] = new("ctm_swat/ctm_swat_variantj", true, "Chem-Haz Specialist | SWAT"),
        [4718] = new("tm_balkan/tm_balkan_variantk", false, "Rezan the Redshirt | Sabre"),
        [4726] = new("tm_professional/tm_professional_varf", false, "Sir Bloody Miami Darryl | The Professionals"),
        [4727] = new("tm_professional/tm_professional_varg", false, "Safecracker Voltzmann | The Professionals"),
        [4728] = new("tm_professional/tm_professional_varh", false, "Little Kev | The Professionals"),
        [4730] = new("tm_professional/tm_professional_varj", false, "Getaway Sally | The Professionals"),
        [4732] = new("tm_professional/tm_professional_vari", false, "Number K | The Professionals"),
        [4733] = new("tm_professional/tm_professional_varf1", false, "Sir Bloody Silent Darryl | The Professionals"),
        [4734] = new("tm_professional/tm_professional_varf2", false, "Sir Bloody Skullhead Darryl | The Professionals"),
        [4735] = new("tm_professional/tm_professional_varf3", false, "Sir Bloody Darryl Royale | The Professionals"),
        [4736] = new("tm_professional/tm_professional_varf4", false, "Sir Bloody Loudmouth Darryl | The Professionals"),
        [4749] = new("ctm_gendarmerie/ctm_gendarmerie_varianta", true, "Sous-Lieutenant Medic | Gendarmerie Nationale"),
        [4750] = new("ctm_gendarmerie/ctm_gendarmerie_variantb", true, "Chem-Haz Capitaine | Gendarmerie Nationale"),
        [4751] = new("ctm_gendarmerie/ctm_gendarmerie_variantc", true, "Chef d'Escadron Rouchard | Gendarmerie Nationale"),
        [4752] = new("ctm_gendarmerie/ctm_gendarmerie_variantd", true, "Aspirant | Gendarmerie Nationale"),
        [4753] = new("ctm_gendarmerie/ctm_gendarmerie_variante", true, "Officer Jacques Beltram | Gendarmerie Nationale"),
        [4756] = new("ctm_swat/ctm_swat_variantk", true, "Lieutenant 'Tree Hugger' Farlow | SWAT"),
        [4757] = new("ctm_diver/ctm_diver_varianta", true, "Cmdr. Davida 'Goggles' Fernandez | SEAL Frogman"),
        [4771] = new("ctm_diver/ctm_diver_variantb", true, "Cmdr. Frank 'Wet Sox' Baroud | SEAL Frogman"),
        [4772] = new("ctm_diver/ctm_diver_variantc", true, "Lieutenant Rex Krikey | SEAL Frogman"),
        [4773] = new("tm_jungle_raider/tm_jungle_raider_varianta", false, "Elite Trapper Solman | Guerrilla Warfare"),
        [4774] = new("tm_jungle_raider/tm_jungle_raider_variantb", false, "Crasswater The Forgotten | Guerrilla Warfare"),
        [4775] = new("tm_jungle_raider/tm_jungle_raider_variantc", false, "Arno The Overgrown | Guerrilla Warfare"),
        [4776] = new("tm_jungle_raider/tm_jungle_raider_variantd", false, "Col. Mangos Dabisi | Guerrilla Warfare"),
        [4777] = new("tm_jungle_raider/tm_jungle_raider_variante", false, "Vypa Sista of the Revolution | Guerrilla Warfare"),
        [4778] = new("tm_jungle_raider/tm_jungle_raider_variantf", false, "Trapper Aggressor | Guerrilla Warfare"),
        [4780] = new("tm_jungle_raider/tm_jungle_raider_variantb2", false, "'Medium Rare' Crasswater | Guerrilla Warfare"),
        [4781] = new("tm_jungle_raider/tm_jungle_raider_variantf2", false, "Trapper | Guerrilla Warfare"),
        [5105] = new("tm_leet/tm_leet_variantg", false, "Ground Rebel | Elite Crew"),
        [5106] = new("tm_leet/tm_leet_varianth", false, "Osiris | Elite Crew"),
        [5107] = new("tm_leet/tm_leet_varianti", false, "Prof. Shahmat | Elite Crew"),
        [5108] = new("tm_leet/tm_leet_variantf", false, "The Elite Mr. Muhlik | Elite Crew"),
        [5109] = new("tm_leet/tm_leet_variantj", false, "Jungle Rebel | Elite Crew"),
        [5205] = new("tm_phoenix/tm_phoenix_varianth", false, "Soldier | Phoenix"),
        [5206] = new("tm_phoenix/tm_phoenix_variantf", false, "Enforcer | Phoenix"),
        [5207] = new("tm_phoenix/tm_phoenix_variantg", false, "Slingshot | Phoenix"),
        [5208] = new("tm_phoenix/tm_phoenix_varianti", false, "Street Soldier | Phoenix"),
        [5305] = new("ctm_fbi/ctm_fbi_variantf", true, "Operator | FBI SWAT"),
        [5306] = new("ctm_fbi/ctm_fbi_variantg", true, "Markus Delrow | FBI HRT"),
        [5307] = new("ctm_fbi/ctm_fbi_varianth", true, "Michael Syfers | FBI Sniper"),
        [5308] = new("ctm_fbi/ctm_fbi_variantb", true, "Special Agent Ava | FBI"),
        [5400] = new("ctm_st6/ctm_st6_variantk", true, "3rd Commando Company | KSK"),
        [5401] = new("ctm_st6/ctm_st6_variante", true, "Seal Team 6 Soldier | NSWC SEAL"),
        [5402] = new("ctm_st6/ctm_st6_variantg", true, "Buckshot | NSWC SEAL"),
        [5403] = new("ctm_st6/ctm_st6_variantm", true, "'Two Times' McCoy | USAF TACP"),
        [5404] = new("ctm_st6/ctm_st6_varianti", true, "Lt. Commander Ricksaw | NSWC SEAL"),
        [5405] = new("ctm_st6/ctm_st6_variantn", true, "Primeiro Tenente | Brazilian 1st Battalion"),
        [5500] = new("tm_balkan/tm_balkan_variantf", false, "Dragomir | Sabre"),
        [5501] = new("tm_balkan/tm_balkan_varianti", false, "Maximus | Sabre"),
        [5502] = new("tm_balkan/tm_balkan_variantg", false, "Rezan The Ready | Sabre"),
        [5503] = new("tm_balkan/tm_balkan_variantj", false, "Blackwolf | Sabre"),
        [5504] = new("tm_balkan/tm_balkan_varianth", false, "'The Doctor' Romanov | Sabre"),
        [5505] = new("tm_balkan/tm_balkan_variantl", false, "Dragomir | Sabre Footsoldier"),
        [5601] = new("ctm_sas/ctm_sas_variantf", true, "B Squadron Officer | SAS"),
        [5602] = new("ctm_sas/ctm_sas_variantg", true, "D Squadron Officer | NZSAS"),
    };

    /// <summary>True if the def index is a known agent (routes to wp_player_agents
    /// instead of wp_player_skins).</summary>
    public static bool IsAgent(uint defIndex) => ByDefIndex.ContainsKey(defIndex);

    /// <summary>Looks up an agent by def index; null if it isn't a known agent.</summary>
    public static AgentInfo? TryGet(uint defIndex) =>
        ByDefIndex.TryGetValue(defIndex, out var info) ? info : null;
}
