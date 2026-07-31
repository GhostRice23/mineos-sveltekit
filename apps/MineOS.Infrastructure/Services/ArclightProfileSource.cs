using System.Text.Json;
using System.Text.RegularExpressions;
using MineOS.Application.Dtos;

namespace MineOS.Infrastructure.Services;

/// <summary>
/// One Arclight build, as described by its release asset filename.
/// </summary>
/// <param name="Loader">forge, neoforge or fabric.</param>
/// <param name="MinecraftVersion">Target Minecraft version, e.g. "1.21.1".</param>
/// <param name="ArclightVersion">Arclight's own version, e.g. "1.0.1".</param>
/// <param name="FileName">The asset filename.</param>
/// <param name="Url">Direct download URL.</param>
/// <param name="PublishedAt">Release publication timestamp.</param>
/// <param name="IsPrerelease">True for SNAPSHOT/prerelease builds.</param>
public record ArclightBuild(
    string Loader,
    string MinecraftVersion,
    string ArclightVersion,
    string FileName,
    string Url,
    string PublishedAt,
    bool IsPrerelease);

/// <summary>
/// Lists Arclight server builds, a Forge/NeoForge/Fabric + Bukkit hybrid.
/// </summary>
/// <remarks>
/// <para>
/// Arclight publishes through GitHub releases, and the asset filename carries
/// both the mod loader and the target Minecraft version:
/// <c>arclight-{loader}-{mcVersion}-{arclightVersion}[-{commit}].jar</c>. Parsing
/// the filename rather than the release tag is deliberate — the tags are
/// codenames (<c>FeudalKings/1.0.1</c>, <c>Trials/1.0.6</c>, <c>horn/1.0.5</c>)
/// that carry no version information, and a single release ships one asset per
/// loader.
/// </para>
/// <para>
/// The two other hybrids commonly asked for alongside Arclight are not here:
/// Mohist serves its build API behind bot protection, so the JSON shape could
/// not be verified and a parser written against a guessed shape would silently
/// yield nothing; and Magma-Neo publishes no GitHub releases at all (its
/// predecessor Magma-Forge last released SNAPSHOT builds in 2022). Both become
/// straightforward to add here once they expose a feed that can be read.
/// </para>
/// </remarks>
public sealed class ArclightProfileSource
{
    /// <summary>GitHub releases feed. 50 covers several years of history.</summary>
    public const string ReleasesUrl = "https://api.github.com/repos/IzzelAliz/Arclight/releases?per_page=50";

    /// <summary>Loaders Arclight builds against, and thus the profile groups produced.</summary>
    public static readonly IReadOnlyList<string> Loaders = new[] { "forge", "neoforge", "fabric" };

    // Anchored so a differently-named asset (sources, checksums, a future
    // variant) is skipped rather than mis-parsed.
    private static readonly Regex AssetPattern = new(
        @"^arclight-(?<loader>forge|neoforge|fabric)-(?<mc>\d+(?:\.\d+)*)-(?<version>.+)\.jar$",
        RegexOptions.Compiled | RegexOptions.IgnoreCase);

    /// <summary>
    /// Extracts the loader, Minecraft version and Arclight version from an asset
    /// filename. Returns null for anything that is not an Arclight server jar.
    /// </summary>
    public static (string Loader, string MinecraftVersion, string ArclightVersion)? ParseAssetName(string? fileName)
    {
        if (string.IsNullOrWhiteSpace(fileName))
            return null;

        var match = AssetPattern.Match(fileName);
        if (!match.Success)
            return null;

        return (
            match.Groups["loader"].Value.ToLowerInvariant(),
            match.Groups["mc"].Value,
            match.Groups["version"].Value);
    }

    /// <summary>
    /// Parses the GitHub releases payload into builds.
    /// </summary>
    /// <param name="json">Response body of <see cref="ReleasesUrl"/>.</param>
    /// <param name="includePrereleases">
    /// Whether to include SNAPSHOT builds. Off by default: they are published
    /// against unreleased Minecraft versions and are not what someone creating a
    /// server wants by default.
    /// </param>
    public static IReadOnlyList<ArclightBuild> ParseReleases(string json, bool includePrereleases = false)
    {
        var builds = new List<ArclightBuild>();

        using var doc = JsonDocument.Parse(json);
        if (doc.RootElement.ValueKind != JsonValueKind.Array)
            return builds;

        foreach (var release in doc.RootElement.EnumerateArray())
        {
            var isPrerelease = release.TryGetProperty("prerelease", out var pre) &&
                               pre.ValueKind == JsonValueKind.True;
            if (isPrerelease && !includePrereleases)
                continue;

            var publishedAt = release.TryGetProperty("published_at", out var published)
                ? published.GetString() ?? string.Empty
                : string.Empty;

            if (!release.TryGetProperty("assets", out var assets) ||
                assets.ValueKind != JsonValueKind.Array)
            {
                continue;
            }

            foreach (var asset in assets.EnumerateArray())
            {
                var name = asset.TryGetProperty("name", out var nameElement) ? nameElement.GetString() : null;
                var url = asset.TryGetProperty("browser_download_url", out var urlElement)
                    ? urlElement.GetString()
                    : null;

                if (string.IsNullOrWhiteSpace(url))
                    continue;

                var parsed = ParseAssetName(name);
                if (parsed is null)
                    continue;

                builds.Add(new ArclightBuild(
                    parsed.Value.Loader,
                    parsed.Value.MinecraftVersion,
                    parsed.Value.ArclightVersion,
                    name!,
                    url!,
                    publishedAt,
                    isPrerelease));
            }
        }

        return builds;
    }

    /// <summary>
    /// Converts builds into profiles, keeping only the newest build per
    /// (loader, Minecraft version).
    /// </summary>
    /// <remarks>
    /// Arclight ships many patch releases against the same Minecraft version;
    /// listing all of them would bury the version picker under near-identical
    /// entries. The releases feed is newest-first, so the first build seen for a
    /// pair is the one kept.
    /// </remarks>
    public static IReadOnlyList<ProfileDto> ToProfiles(IEnumerable<ArclightBuild> builds)
    {
        var newestPerTarget = new Dictionary<string, ArclightBuild>(StringComparer.OrdinalIgnoreCase);

        foreach (var build in builds)
        {
            var key = $"{build.Loader}-{build.MinecraftVersion}";
            if (!newestPerTarget.ContainsKey(key))
                newestPerTarget[key] = build;
        }

        return newestPerTarget.Values
            .OrderBy(b => b.Loader, StringComparer.Ordinal)
            .ThenByDescending(b => ParseVersionOrZero(b.MinecraftVersion))
            .Select(b => new ProfileDto(
                // Id is stable across refreshes so a downloaded profile keeps
                // being recognised as downloaded.
                $"arclight-{b.Loader}-{b.MinecraftVersion}",
                $"arclight-{b.Loader}",
                b.IsPrerelease ? "snapshot" : "release",
                b.MinecraftVersion,
                b.PublishedAt,
                b.Url,
                b.FileName,
                false,
                null))
            .ToList();
    }

    private static Version ParseVersionOrZero(string version)
        => Version.TryParse(version, out var parsed) ? parsed : new Version(0, 0);
}
