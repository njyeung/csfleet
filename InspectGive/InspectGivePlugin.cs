using CounterStrikeSharp.API;
using CounterStrikeSharp.API.Core;
using CounterStrikeSharp.API.Modules.Commands;
using Microsoft.Extensions.Logging;

namespace InspectGive;

/// <summary>
/// Lets a player paste a CS2 inspect command into chat — the
/// <c>csgo_econ_action_preview &lt;hex&gt;</c> line copied from cs2inspects.com — and
/// have that exact skin applied to their loadout via the WeaponPaints database.
/// See docs/cs2-inspect-codes.md §6.
///
/// Flow: intercept the chat line → parse the blob → upsert wp_player_skins (T+CT)
/// off-thread → back on the main thread fire WeaponPaints' server-console
/// <c>wp_refresh &lt;steamid&gt;</c> to re-apply.
/// </summary>
public sealed class InspectGivePlugin : BasePlugin, IPluginConfig<InspectGiveConfig>
{
    public override string ModuleName => "InspectGive";
    public override string ModuleVersion => "0.3.0";
    public override string ModuleAuthor => "cs2-server";
    public override string ModuleDescription => "Apply skins from CS2 inspect blobs via WeaponPaints.";

    public InspectGiveConfig Config { get; set; } = new();
    private DatabaseHandler _db = null!;

    private const int StepDelayMs = 100;
    private const int MaxChatMessageLength = 127;

    public void OnConfigParsed(InspectGiveConfig config)
    {
        Config = config;
        _db = new DatabaseHandler(config);
    }

    public override void Load(bool hotReload)
    {
        // A chat message starting with the inspect prefix is intercepted and applied
        // directly; we swallow it so the long blob doesn't broadcast to everyone.
        AddCommandListener("say", OnSayInspect, HookMode.Pre);
        AddCommandListener("say_team", OnSayInspect, HookMode.Pre);

        // Console command (`~`): the reliable path for full-length blobs, since the
        // console accepts ~512 chars where chat caps far shorter.
        AddCommand(Config.Command, "Apply a skin from a CS2 inspect blob", OnInspectCommand);
    }

    private HookResult OnSayInspect(CCSPlayerController? player, CommandInfo info)
    {
        if (player is null || !player.IsValid)
            return HookResult.Continue;

        var msg = info.GetArg(1).Trim();
        if (!msg.StartsWith(Config.ChatTriggerPrefix, StringComparison.OrdinalIgnoreCase))
            return HookResult.Continue;

        ApplyInspect(player, msg, fromChat: true);
        return HookResult.Handled;
    }

    private void OnInspectCommand(CCSPlayerController? player, CommandInfo info)
    {
        if (player is null || !player.IsValid)
            return;
        
        var arg = info.ArgString.Trim();

        if (!arg.StartsWith(Config.ChatTriggerPrefix, StringComparison.OrdinalIgnoreCase))
        {
            player.PrintToChat($" \x02[InspectGive]\x01 Usage: {Config.Command} {Config.ChatTriggerPrefix} <hex>");
            return;
        }

        ApplyInspect(player, arg);
    }

    /// <summary>Decodes the blob and applies the skin to the player's loadout. Both the
    /// chat listener and the console command funnel through here. <paramref name="fromChat"/>
    /// lets us give a "use console" hint when a chat paste was truncated.</summary>
    private void ApplyInspect(CCSPlayerController player, string blob, bool fromChat = false)
    {
        InspectItem item;
        try
        {
            item = InspectDecoder.Parse(blob);
        }
        catch (FormatException ex)
        {
            Logger.LogWarning("Inspect parse failed (fromChat={fromChat}, len={len}): {msg} | input=\"{blob}\"",
                fromChat, blob.Length, ex.Message, blob);

            // A chat paste sitting exactly at CS2's 127-char cap was probably cut
            // off mid-blob.

            if (fromChat && blob.Length >= MaxChatMessageLength)
                player.PrintToChat(
                    $" \x02[InspectGive] That inspect got cut off - chat is capped at {MaxChatMessageLength} characters. " +
                    $"Open console (~) and paste: \x06{Config.Command} {Config.ChatTriggerPrefix} <hex>");
            else
                player.PrintToChat($" \x02[InspectGive]\x01 Couldn't read that inspect: {ex.Message}");
            return;
        }

        var steamId = player.SteamID;
        var slot = player.Slot;

        Logger.LogInformation("Inspect from {steamId}: {item}", steamId, item);

        // Agents aren't weapons so route them separately.
        if (AgentNames.TryGet(item.DefIndex) is { } agent)
        {
            ApplyAgent(player, agent, steamId, slot);
            return;
        }

        player.PrintToChat($" \x04[InspectGive]\x01 Applying \x06{SkinNames.Describe(item)}\x01 ...");

        Task.Run(async () =>
        {
            try
            {
                await _db.ApplyAsync(steamId, item, Config.ApplyToBothTeams);
                Logger.LogInformation("DB write OK for {steamId} (def {def})", steamId, item.DefIndex);
                // Small breather so WeaponPaints reads the committed rows, not a stale cache.
                await Task.Delay(StepDelayMs);
                Server.NextFrame(() => ApplyRefresh(slot, steamId, item));
            }
            catch (Exception ex)
            {
                Logger.LogError(ex, "InspectGive DB write failed for {steamId}", steamId);
                Server.NextFrame(() =>
                {
                    var p = Utilities.GetPlayerFromSlot(slot);
                    if (p is { IsValid: true })
                        p.PrintToChat(" \x02[InspectGive]\x01 Database error — skin not applied.");
                });
            }
        });
    }

    /// <summary>Writes an inspected agent to wp_player_agents off-thread, then fires
    /// wp_refresh on the main thread to swap the player model. Mirrors <see cref="ApplyInspect"/>
    /// but with no weapon-give step — agents aren't given items.</summary>
    private void ApplyAgent(CCSPlayerController player, AgentInfo agent, ulong steamId, int slot)
    {
        player.PrintToChat($" \x04[InspectGive]\x01 Applying agent \x06{agent.Name}\x01 …");

        Task.Run(async () =>
        {
            try
            {
                await _db.ApplyAgentAsync(steamId, agent, Config.ApplyToBothTeams);
                Logger.LogInformation("Agent DB write OK for {steamId} (slug {slug})", steamId, agent.Slug);
                await Task.Delay(StepDelayMs);
                Server.NextFrame(() =>
                {
                    Logger.LogInformation("Executing wp_refresh {steamId} (agent)", steamId);
                    Server.ExecuteCommand($"wp_refresh {steamId}");
                    var p = Utilities.GetPlayerFromSlot(slot);
                    if (p is { IsValid: true } && p.SteamID == steamId)
                        p.PrintToChat(" \x04[InspectGive]\x01 Agent applied! Respawn if the model doesn't update.");
                });
            }
            catch (Exception ex)
            {
                Logger.LogError(ex, "InspectGive agent DB write failed for {steamId}", steamId);
                Server.NextFrame(() =>
                {
                    var p = Utilities.GetPlayerFromSlot(slot);
                    if (p is { IsValid: true })
                        p.PrintToChat(" \x02[InspectGive]\x01 Database error — agent not applied.");
                });
            }
        });
    }

    private void ApplyRefresh(int slot, ulong steamId, InspectItem item)
    {
        // The DB write is already committed (we awaited it), so WeaponPaints'
        // wp_refresh will read the new rows and re-apply the loadout.
        Logger.LogInformation("Executing wp_refresh {steamId}", steamId);
        Server.ExecuteCommand($"wp_refresh {steamId}");

        // Let wp_refresh land before handing over the weapon, so the freshly given
        // weapon spawns with the skin already in WeaponPaints' cache.
        AddTimer(StepDelayMs / 1000f, () => GiveWeapon(slot, steamId, item));
    }

    private void GiveWeapon(int slot, ulong steamId, InspectItem item)
    {
        var p = Utilities.GetPlayerFromSlot(slot);
        if (p is not { IsValid: true } || p.SteamID != steamId)
            return;

        // Hand the weapon over for free.
        if (TryGiveWeapon(p, item))
            p.PrintToChat(" \x04[InspectGive]\x01 Skin applied and weapon given! Enjoy.");
        else
            p.PrintToChat(" \x04[InspectGive]\x01 Skin applied! Re-equip the weapon if it doesn't update.");
    }

    /// <summary>Gives the player the weapon matching the inspect's def index. Returns
    /// false (and gives nothing) if the player is dead or the def index isn't a known
    /// buyable gun — the skin is still applied to their loadout either way.
    ///
    /// Knives and gloves are deliberately not handed over: the player always already has
    /// a knife (and gloves aren't a weapon entity), so WeaponPaints' wp_refresh swaps
    /// those models in place. Giving a knife entity here would just duplicate it.</summary>
    private bool TryGiveWeapon(CCSPlayerController player, InspectItem item)
    {
        if (KnifeNames.IsKnife(item.DefIndex) || GloveNames.IsGlove(item.DefIndex))
            return false;

        var weaponName = WeaponNames.TryGet(item.DefIndex);
        if (weaponName is null)
            return false;

        // GiveNamedItem needs a live pawn with item services; dead players have none.
        var pawn = player.PlayerPawn.Value;
        if (pawn is null || !pawn.IsValid
            || pawn.LifeState != (byte)LifeState_t.LIFE_ALIVE
            || pawn.ItemServices is null)
            return false;

        // GiveNamedItem lives on the CS-specific item services, not the base type.
        new CCSPlayer_ItemServices(pawn.ItemServices.Handle).GiveNamedItem<CBasePlayerWeapon>(weaponName);
        Logger.LogInformation("Gave {weapon} (def {def}) to {steamId}", weaponName, item.DefIndex, player.SteamID);
        return true;
    }
}
