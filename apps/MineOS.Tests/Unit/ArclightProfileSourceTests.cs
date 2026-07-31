using MineOS.Infrastructure.Services;

namespace MineOS.Tests.Unit;

public class ArclightProfileSourceTests
{
    /// <summary>
    /// Trimmed to the fields the parser reads, but the tag names, asset names
    /// and prerelease flags are copied from the real releases feed — including
    /// the codename tags that carry no version, which is why the asset filename
    /// is what gets parsed.
    /// </summary>
    private const string ReleasesJson = """
    [
      {
        "tag_name": "FeudalKings/1.0.1",
        "prerelease": false,
        "published_at": "2026-03-02T10:00:00Z",
        "assets": [
          { "name": "arclight-fabric-1.21.1-1.0.1-8ec9529.jar",
            "browser_download_url": "https://github.com/IzzelAliz/Arclight/releases/download/FeudalKings/1.0.1/arclight-fabric-1.21.1-1.0.1-8ec9529.jar" },
          { "name": "arclight-forge-1.21.1-1.0.1-8ec9529.jar",
            "browser_download_url": "https://github.com/IzzelAliz/Arclight/releases/download/FeudalKings/1.0.1/arclight-forge-1.21.1-1.0.1-8ec9529.jar" },
          { "name": "arclight-neoforge-1.21.1-1.0.1-8ec9529.jar",
            "browser_download_url": "https://github.com/IzzelAliz/Arclight/releases/download/FeudalKings/1.0.1/arclight-neoforge-1.21.1-1.0.1-8ec9529.jar" }
        ]
      },
      {
        "tag_name": "FeudalKings/1.0.0",
        "prerelease": false,
        "published_at": "2026-02-01T10:00:00Z",
        "assets": [
          { "name": "arclight-fabric-1.21.1-1.0.0-3457560.jar",
            "browser_download_url": "https://example.invalid/arclight-fabric-1.21.1-1.0.0-3457560.jar" }
        ]
      },
      {
        "tag_name": "FeudalKings/1.0.0-SNAPSHOT",
        "prerelease": true,
        "published_at": "2026-01-15T10:00:00Z",
        "assets": [
          { "name": "arclight-fabric-1.21-1.0.0-SNAPSHOT.jar",
            "browser_download_url": "https://example.invalid/arclight-fabric-1.21-1.0.0-SNAPSHOT.jar" }
        ]
      },
      {
        "tag_name": "Trials/1.0.6",
        "prerelease": false,
        "published_at": "2025-11-20T10:00:00Z",
        "assets": [
          { "name": "arclight-forge-1.20.1-1.0.6.jar",
            "browser_download_url": "https://example.invalid/arclight-forge-1.20.1-1.0.6.jar" }
        ]
      },
      {
        "tag_name": "download",
        "prerelease": false,
        "published_at": "2025-01-01T10:00:00Z",
        "assets": []
      }
    ]
    """;

    [Theory]
    [InlineData("arclight-forge-1.21.1-1.0.1-8ec9529.jar", "forge", "1.21.1", "1.0.1-8ec9529")]
    [InlineData("arclight-neoforge-1.21.1-1.0.1-8ec9529.jar", "neoforge", "1.21.1", "1.0.1-8ec9529")]
    [InlineData("arclight-fabric-1.20.4-1.0.3-13f0d63.jar", "fabric", "1.20.4", "1.0.3-13f0d63")]
    [InlineData("arclight-forge-1.18.2-1.0.12.jar", "forge", "1.18.2", "1.0.12")]
    [InlineData("arclight-fabric-1.21-1.0.0-SNAPSHOT.jar", "fabric", "1.21", "1.0.0-SNAPSHOT")]
    public void ParseAssetName_ReadsLoaderAndMinecraftVersion(
        string fileName, string loader, string mcVersion, string arclightVersion)
    {
        var parsed = ArclightProfileSource.ParseAssetName(fileName);

        Assert.NotNull(parsed);
        Assert.Equal(loader, parsed!.Value.Loader);
        Assert.Equal(mcVersion, parsed.Value.MinecraftVersion);
        Assert.Equal(arclightVersion, parsed.Value.ArclightVersion);
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("arclight-1.21.1-1.0.1.jar")]          // no loader
    [InlineData("arclight-quilt-1.21.1-1.0.1.jar")]    // loader Arclight does not build
    [InlineData("checksums.txt")]
    [InlineData("arclight-forge-1.21.1-1.0.1.zip")]    // not a jar
    [InlineData("something-forge-1.21.1-1.0.1.jar")]   // not Arclight
    public void ParseAssetName_RejectsAnythingThatIsNotAnArclightJar(string? fileName)
    {
        // Anchored matching, so an unexpected asset is skipped rather than
        // mis-parsed into a broken profile.
        Assert.Null(ArclightProfileSource.ParseAssetName(fileName));
    }

    [Fact]
    public void ParseReleases_SkipsPrereleasesByDefault()
    {
        var builds = ArclightProfileSource.ParseReleases(ReleasesJson);

        Assert.DoesNotContain(builds, b => b.IsPrerelease);
        // The 1.21 SNAPSHOT build targets a version no stable build covers, so
        // its absence is observable.
        Assert.DoesNotContain(builds, b => b.MinecraftVersion == "1.21");
    }

    [Fact]
    public void ParseReleases_CanIncludePrereleases()
    {
        var builds = ArclightProfileSource.ParseReleases(ReleasesJson, includePrereleases: true);

        Assert.Contains(builds, b => b.IsPrerelease && b.MinecraftVersion == "1.21");
    }

    [Fact]
    public void ParseReleases_ReadsEveryLoaderInARelease()
    {
        var builds = ArclightProfileSource.ParseReleases(ReleasesJson);

        var loaders = builds
            .Where(b => b.MinecraftVersion == "1.21.1")
            .Select(b => b.Loader)
            .OrderBy(l => l)
            .ToArray();

        // A single release ships one asset per loader.
        Assert.Equal(new[] { "fabric", "forge", "neoforge" }, loaders);
    }

    [Fact]
    public void ParseReleases_IgnoresReleasesWithoutAssets()
    {
        var builds = ArclightProfileSource.ParseReleases(ReleasesJson);

        Assert.DoesNotContain(builds, b => string.IsNullOrEmpty(b.Url));
    }

    [Fact]
    public void ParseReleases_ToleratesAnEmptyOrUnexpectedPayload()
    {
        Assert.Empty(ArclightProfileSource.ParseReleases("[]"));
        Assert.Empty(ArclightProfileSource.ParseReleases("{}"));
        Assert.Empty(ArclightProfileSource.ParseReleases("""[{"prerelease": false}]"""));
    }

    [Fact]
    public void ToProfiles_KeepsOnlyTheNewestBuildPerLoaderAndVersion()
    {
        var builds = ArclightProfileSource.ParseReleases(ReleasesJson);

        var profiles = ArclightProfileSource.ToProfiles(builds);

        // Two stable fabric builds target 1.21.1; only the newer one is offered,
        // otherwise the picker fills up with near-identical entries.
        var fabric1211 = profiles.Where(p => p.Id == "arclight-fabric-1.21.1").ToArray();
        var single = Assert.Single(fabric1211);
        Assert.Contains("1.0.1", single.Url);
    }

    [Fact]
    public void ToProfiles_GroupsByLoaderSoThePickerCanFilter()
    {
        var profiles = ArclightProfileSource.ToProfiles(ArclightProfileSource.ParseReleases(ReleasesJson));

        Assert.Contains(profiles, p => p.Group == "arclight-forge");
        Assert.Contains(profiles, p => p.Group == "arclight-neoforge");
        Assert.Contains(profiles, p => p.Group == "arclight-fabric");
    }

    [Fact]
    public void ToProfiles_UsesAStableIdSoDownloadsStayRecognised()
    {
        var first = ArclightProfileSource.ToProfiles(ArclightProfileSource.ParseReleases(ReleasesJson));
        var second = ArclightProfileSource.ToProfiles(ArclightProfileSource.ParseReleases(ReleasesJson));

        Assert.Equal(first.Select(p => p.Id), second.Select(p => p.Id));
        // The id must not carry the build hash, or every new build would look
        // like a different profile and lose its "downloaded" state.
        Assert.All(first, p => Assert.DoesNotContain("8ec9529", p.Id));
    }

    [Fact]
    public void ToProfiles_OrdersNewestMinecraftVersionFirstWithinALoader()
    {
        var profiles = ArclightProfileSource.ToProfiles(ArclightProfileSource.ParseReleases(ReleasesJson));

        var forgeVersions = profiles
            .Where(p => p.Group == "arclight-forge")
            .Select(p => p.Version)
            .ToArray();

        Assert.Equal(new[] { "1.21.1", "1.20.1" }, forgeVersions);
    }

    [Fact]
    public void ToProfiles_CarriesTheDownloadUrlAndFilename()
    {
        var profiles = ArclightProfileSource.ToProfiles(ArclightProfileSource.ParseReleases(ReleasesJson));

        var forge = Assert.Single(profiles, p => p.Id == "arclight-forge-1.21.1");
        Assert.Equal("arclight-forge-1.21.1-1.0.1-8ec9529.jar", forge.Filename);
        Assert.StartsWith("https://github.com/IzzelAliz/Arclight/releases/download/", forge.Url);
        Assert.Equal("release", forge.Type);
        Assert.Equal("2026-03-02T10:00:00Z", forge.ReleaseTime);
    }

    [Fact]
    public void ToProfiles_OfEmptyInputIsEmpty()
    {
        Assert.Empty(ArclightProfileSource.ToProfiles(Array.Empty<ArclightBuild>()));
    }
}
