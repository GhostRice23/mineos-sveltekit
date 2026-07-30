using MineOS.Infrastructure.Services;

namespace MineOS.Tests.Unit;

public class JavaRuntimeServiceTests
{
    /// <summary>
    /// Builds a service over a fake JVM root, so discovery is exercised without
    /// a JDK installed on the machine running the suite.
    /// </summary>
    private static JavaRuntimeService ServiceWith(params string[] directories)
    {
        var dirs = directories.Select(d => $"/usr/lib/jvm/{d}").ToArray();
        var executables = dirs.Select(d => Path.Combine(d, "bin", "java")).ToHashSet();

        return new JavaRuntimeService(
            "/usr/lib/jvm",
            executables.Contains,
            _ => dirs);
    }

    [Theory]
    [InlineData("temurin-21-jdk-amd64", 21)]
    [InlineData("temurin-8-jre", 8)]
    [InlineData("java-17-openjdk-amd64", 17)]
    [InlineData("java-25-openjdk", 25)]
    [InlineData("zulu-11", 11)]
    [InlineData("temurin-21-jdk-arm64", 21)]
    public void ParseMajorVersion_ReadsTheFeatureVersion(string directory, int expected)
    {
        Assert.Equal(expected, JavaRuntimeService.ParseMajorVersion(directory));
    }

    [Theory]
    [InlineData("java-1.8.0-openjdk-amd64", 8)]
    public void ParseMajorVersion_IgnoresTheLegacyOnePrefix(string directory, int expected)
    {
        // "1.8.0" means Java 8 — the leading 1 is the old version scheme, not
        // a feature version.
        Assert.Equal(expected, JavaRuntimeService.ParseMajorVersion(directory));
    }

    [Theory]
    [InlineData("")]
    [InlineData("default-java")]
    [InlineData("some-jdk")]
    public void ParseMajorVersion_ReturnsNullWhenThereIsNoVersion(string directory)
    {
        Assert.Null(JavaRuntimeService.ParseMajorVersion(directory));
    }

    [Fact]
    public void Discover_ListsOnlyDirectoriesThatActuallyHaveAJavaExecutable()
    {
        var service = new JavaRuntimeService(
            "/usr/lib/jvm",
            path => path == "/usr/lib/jvm/temurin-21-jdk-amd64/bin/java",
            _ => new[] { "/usr/lib/jvm/temurin-21-jdk-amd64", "/usr/lib/jvm/broken-install" });

        var runtimes = service.Discover();

        var runtime = Assert.Single(runtimes);
        Assert.Equal("/usr/lib/jvm/temurin-21-jdk-amd64/bin/java", runtime.Path);
        Assert.Equal(21, runtime.MajorVersion);
    }

    [Fact]
    public void Discover_OrdersNewestFirst()
    {
        var service = ServiceWith("temurin-8-jre", "temurin-21-jdk-amd64", "java-17-openjdk-amd64");

        var versions = service.Discover().Select(r => r.MajorVersion).ToArray();

        Assert.Equal(new int?[] { 21, 17, 8 }, versions);
    }

    [Fact]
    public void Discover_LabelsRuntimesForAPicker()
    {
        var service = ServiceWith("temurin-21-jdk-amd64");

        var runtime = Assert.Single(service.Discover());

        Assert.Equal("Java 21 (temurin-21-jdk-amd64)", runtime.Label);
    }

    [Fact]
    public void Discover_FallsBackToTheDirectoryNameWhenTheVersionIsUnknown()
    {
        var service = ServiceWith("default-java");

        var runtime = Assert.Single(service.Discover());

        Assert.Null(runtime.MajorVersion);
        Assert.Equal("default-java", runtime.Label);
    }

    [Fact]
    public void Discover_ReturnsEmptyWhenNothingIsInstalled()
    {
        var service = new JavaRuntimeService("/usr/lib/jvm", _ => false, _ => Array.Empty<string>());

        Assert.Empty(service.Discover());
    }

    [Theory]
    [InlineData("1.16.5", new[] { 8 })]
    [InlineData("1.17", new[] { 21, 17 })]
    [InlineData("1.20.4", new[] { 21, 17 })]
    [InlineData("1.21.11", new[] { 21 })]
    [InlineData("26.1", new[] { 25, 21 })]
    public void PreferredMajorVersions_MapsMinecraftVersionsToJavaVersions(string mcVersion, int[] expected)
    {
        Assert.Equal(expected, JavaRuntimeService.PreferredMajorVersions(mcVersion));
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("not-a-version")]
    public void PreferredMajorVersions_HasNoOpinionForAnUnknownVersion(string? mcVersion)
    {
        Assert.Empty(JavaRuntimeService.PreferredMajorVersions(mcVersion));
    }

    [Fact]
    public void ResolveForMinecraftVersion_PicksTheInstalledPreferredRuntime()
    {
        var service = ServiceWith("temurin-8-jre", "temurin-21-jdk-amd64");

        Assert.Equal("/usr/lib/jvm/temurin-21-jdk-amd64/bin/java", service.ResolveForMinecraftVersion("1.21.4"));
        Assert.Equal("/usr/lib/jvm/temurin-8-jre/bin/java", service.ResolveForMinecraftVersion("1.16.5"));
    }

    [Fact]
    public void ResolveForMinecraftVersion_HonoursThePreferenceOrder()
    {
        // 1.17-1.20 prefers 21 and accepts 17.
        var both = ServiceWith("java-17-openjdk-amd64", "temurin-21-jdk-amd64");
        Assert.Equal("/usr/lib/jvm/temurin-21-jdk-amd64/bin/java", both.ResolveForMinecraftVersion("1.18.2"));

        var only17 = ServiceWith("java-17-openjdk-amd64");
        Assert.Equal("/usr/lib/jvm/java-17-openjdk-amd64/bin/java", only17.ResolveForMinecraftVersion("1.18.2"));
    }

    [Fact]
    public void ResolveForMinecraftVersion_FallsBackToPathWhenNothingSuitableIsInstalled()
    {
        // Deferring to PATH is the safe answer: naming a path that does not
        // exist would fail at launch with a confusing error.
        var service = ServiceWith("temurin-8-jre");

        Assert.Equal("java", service.ResolveForMinecraftVersion("1.21.4"));
        Assert.Equal("java", service.ResolveForMinecraftVersion(null));
    }
}
