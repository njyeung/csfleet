using System.Globalization;

namespace InspectGive;

/// <summary>
/// The skin data carried by a <c>csgo_econ_action_preview</c> inspect blob — the
/// decoded <c>CEconItemPreviewDataBlock</c> protobuf. See docs/cs2-inspect-codes.md §4.
/// </summary>
public sealed class InspectItem
{
    public uint DefIndex { get; init; }
    public uint PaintIndex { get; init; }
    public float Wear { get; init; }
    public uint Seed { get; init; }
    public bool StatTrak { get; init; }
    public uint StatTrakCount { get; init; }
    public string NameTag { get; init; } = string.Empty;
    public IReadOnlyList<InspectSticker> Stickers { get; init; } = Array.Empty<InspectSticker>();
    public InspectKeychain? Keychain { get; init; }

    public override string ToString() =>
        $"def={DefIndex} paint={PaintIndex} wear={Wear:0.######} seed={Seed}" +
        (StatTrak ? $" StatTrak({StatTrakCount})" : "") +
        (NameTag.Length > 0 ? $" \"{NameTag}\"" : "") +
        (Stickers.Count > 0 ? $" stickers={Stickers.Count}" : "") +
        (Keychain is not null ? $" charm={Keychain.Id}" : "");
}

/// <summary>
/// A sticker applied to a weapon, from a repeated proto field 12 sub-message.
/// The proto's slot/position field is intentionally not read: in CS2 players place
/// stickers freely, and WeaponPaints applies them by list index regardless of slot,
/// so only the count and per-sticker placement (offsets/rotation/wear) matter.
/// </summary>
public sealed class InspectSticker
{
    public uint Id { get; init; }
    public float Wear { get; init; }
    public float Scale { get; init; }
    public float Rotation { get; init; }
    public float OffsetX { get; init; }
    public float OffsetY { get; init; }
}

/// <summary>
/// A charm ("keychain") hung off a weapon, from proto field 20. Reuses the same
/// sub-message layout as a sticker, so the offsets live in fields 7/8/9 and the
/// pattern seed in field 10.
/// </summary>
public sealed class InspectKeychain
{
    public uint Id { get; init; }
    public float OffsetX { get; init; }
    public float OffsetY { get; init; }
    public float OffsetZ { get; init; }
    public uint Seed { get; init; }
}

/// <summary>
/// Decodes the <c>csgo_econ_action_preview</c> hex blob into an <see cref="InspectItem"/>.
///
/// Wire format: <c>[0x00 prefix] + [varint protobuf] + [CRC32, 4 bytes]</c>.
/// The scalar fields, plus the repeated sticker (12) and keychain/charm (20)
/// sub-messages and the customname string, are read; anything else is skipped.
/// </summary>
public static class InspectDecoder
{
    // Proto field numbers (CEconItemPreviewDataBlock).
    private const int FieldDefIndex = 3;
    private const int FieldPaintIndex = 4;
    private const int FieldPaintWear = 7;   // uint32 holding the float32 bit pattern
    private const int FieldPaintSeed = 8;
    private const int FieldKillEaterType = 9;   // presence ⇒ StatTrak
    private const int FieldKillEaterValue = 10;
    private const int FieldCustomName = 11;
    private const int FieldStickers = 12;    // repeated Sticker sub-message
    private const int FieldKeychains = 20;   // repeated Sticker sub-message (charms)

    /// <summary>
    /// Pulls the hex token out of whatever the user pasted and decodes it. Accepts a
    /// bare hex string, or a full <c>csgo_econ_action_preview &lt;hex&gt;</c> command
    /// (we take the last whitespace-delimited chunk). Throws <see cref="FormatException"/>
    /// on malformed input.
    /// </summary>
    public static InspectItem Parse(string input)
    {
        if (string.IsNullOrWhiteSpace(input))
            throw new FormatException("empty inspect input");

        // Last whitespace chunk = the hex blob; strips any leading command word.
        var token = input.Trim().Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries)[^1]
            .Trim('"', '\'');

        var bytes = HexDecode(token);
        return Decode(bytes);
    }

    private static byte[] HexDecode(string hex)
    {
        if (hex.Length % 2 != 0)
            throw new FormatException($"hex length is odd ({hex.Length})");

        var bytes = new byte[hex.Length / 2];
        for (var i = 0; i < bytes.Length; i++)
        {
            if (!byte.TryParse(hex.AsSpan(i * 2, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out bytes[i]))
                throw new FormatException($"not valid hex near offset {i * 2}");
        }
        return bytes;
    }

    private static InspectItem Decode(byte[] bytes)
    {
        // Drop the optional 0x00 prefix and the trailing 4-byte CRC32.
        var start = bytes.Length > 0 && bytes[0] == 0x00 ? 1 : 0;
        var end = bytes.Length - 4;
        if (end <= start)
            throw new FormatException($"blob too short ({bytes.Length} bytes)");

        uint defIndex = 0, paintIndex = 0, seed = 0, wearBits = 0, stCount = 0;
        var statTrak = false;
        var nameTag = string.Empty;
        var stickers = new List<InspectSticker>();
        InspectKeychain? keychain = null;

        var i = start;
        while (i < end)
        {
            var tag = ReadVarint(bytes, ref i, end);
            var field = (int)(tag >> 3);
            var wire = (int)(tag & 0x7);

            switch (wire)
            {
                case 0: // varint
                    var v = ReadVarint(bytes, ref i, end);
                    switch (field)
                    {
                        case FieldDefIndex: defIndex = (uint)v; break;
                        case FieldPaintIndex: paintIndex = (uint)v; break;
                        case FieldPaintWear: wearBits = (uint)v; break;
                        case FieldPaintSeed: seed = (uint)v; break;
                        case FieldKillEaterType: statTrak = true; break;
                        case FieldKillEaterValue: stCount = (uint)v; break;
                    }
                    break;

                case 2: // length-delimited (stickers / keychains / strings)
                    var len = (int)ReadVarint(bytes, ref i, end);
                    if (len < 0 || i + len > end)
                        throw new FormatException("length-delimited field overruns blob");
                    switch (field)
                    {
                        case FieldCustomName:
                            nameTag = System.Text.Encoding.UTF8.GetString(bytes, i, len);
                            break;
                        case FieldStickers:
                            var sk = ParseSub(bytes, i, i + len);
                            if (sk.Id != 0)
                                stickers.Add(new InspectSticker
                                {
                                    Id = sk.Id,
                                    Wear = sk.Wear,
                                    Scale = sk.Scale,
                                    Rotation = sk.Rotation,
                                    OffsetX = sk.OffsetX,
                                    OffsetY = sk.OffsetY,
                                });
                            break;
                        case FieldKeychains:
                            var kc = ParseSub(bytes, i, i + len);
                            // Only the last charm matters since a weapon carries at most one.
                            if (kc.Id != 0)
                                keychain = new InspectKeychain
                                {
                                    Id = kc.Id,
                                    OffsetX = kc.OffsetX,
                                    OffsetY = kc.OffsetY,
                                    OffsetZ = kc.OffsetZ,
                                    Seed = kc.Pattern,
                                };
                            break;
                    }
                    i += len;
                    break;

                case 5: // fixed32
                    i += 4;
                    break;

                case 1: // fixed64
                    i += 8;
                    break;

                default:
                    throw new FormatException($"unsupported wire type {wire} for field {field}");
            }
        }

        if (defIndex == 0)
            throw new FormatException("no defindex in blob — not a weapon inspect");

        return new InspectItem
        {
            DefIndex = defIndex,
            PaintIndex = paintIndex,
            // paintwear is the float32 bit pattern stored in a uint32.
            Wear = BitConverter.Int32BitsToSingle((int)wearBits),
            Seed = seed,
            StatTrak = statTrak,
            StatTrakCount = stCount,
            NameTag = nameTag,
            Stickers = stickers,
            Keychain = keychain,
        };
    }

    /// <summary>The fields shared by the sticker (12) and keychain (20) sub-messages.
    /// Both proto messages use the same layout — slot(1), id(2), wear(3), scale(4),
    /// rotation(5), then offset x/y/z in 7/8/9 and a pattern/seed in 10 — so one
    /// reader covers both. Unread fields (slot 1, tint_id 6, etc.) are skipped by wire type.</summary>
    private struct Sub
    {
        public uint Id;
        public float Wear;
        public float Scale;
        public float Rotation;
        public float OffsetX;
        public float OffsetY;
        public float OffsetZ;
        public uint Pattern;
    }

    private static Sub ParseSub(byte[] bytes, int start, int end)
    {
        // Scale defaults to 1.0: when a sticker is at its default size the field is
        // omitted, and writing 0 would shrink it to nothing in WeaponPaints.
        var s = new Sub { Scale = 1f };
        var i = start;
        while (i < end)
        {
            var tag = ReadVarint(bytes, ref i, end);
            var field = (int)(tag >> 3);
            var wire = (int)(tag & 0x7);
            switch (wire)
            {
                case 0: // varint: slot (ignored, we just smush them) / id / tint_id / pattern
                    var v = ReadVarint(bytes, ref i, end);
                    switch (field)
                    {
                        case 2: s.Id = (uint)v; break;
                        case 10: s.Pattern = (uint)v; break;
                    }
                    break;

                case 5: // fixed32: float wear / scale / rotation / offsets
                    var f = BitConverter.Int32BitsToSingle((int)ReadFixed32(bytes, ref i, end));
                    switch (field)
                    {
                        case 3: s.Wear = f; break;
                        case 4: s.Scale = f; break;
                        case 5: s.Rotation = f; break;
                        case 7: s.OffsetX = f; break;
                        case 8: s.OffsetY = f; break;
                        case 9: s.OffsetZ = f; break;
                    }
                    break;

                case 1: // fixed64
                    i += 8;
                    break;

                case 2: // nested length-delimited: skip
                    var len = (int)ReadVarint(bytes, ref i, end);
                    if (len < 0 || i + len > end)
                        throw new FormatException("nested field overruns sub-message");
                    i += len;
                    break;

                default:
                    throw new FormatException($"unsupported wire type {wire} for sub-field {field}");
            }
        }
        return s;
    }

    private static uint ReadFixed32(byte[] bytes, ref int i, int end)
    {
        if (i + 4 > end)
            throw new FormatException("truncated fixed32");
        var v = (uint)(bytes[i] | bytes[i + 1] << 8 | bytes[i + 2] << 16 | bytes[i + 3] << 24);
        i += 4;
        return v;
    }

    private static ulong ReadVarint(byte[] bytes, ref int i, int end)
    {
        ulong result = 0;
        var shift = 0;
        while (i < end)
        {
            var b = bytes[i++];
            result |= (ulong)(b & 0x7F) << shift;
            if ((b & 0x80) == 0)
                return result;
            shift += 7;
            if (shift >= 64)
                throw new FormatException("varint too long");
        }
        throw new FormatException("truncated varint");
    }
}
