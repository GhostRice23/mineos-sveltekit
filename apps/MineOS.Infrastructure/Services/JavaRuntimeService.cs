using MineOS.Application.Interfaces;

namespace MineOS.Infrastructure.Services;

/// <summary>
/// Finds Java runtimes by scanning the JVM install root.
/// </summary>
/// <remarks>
/// Scanning beats the previous approach of probing a fixed list of paths: the
/// container image can add or move a JDK without this needing to know the exact
/// directory name, and the UI can offer exactly what is actually installed.
/// </remarks>
public sealed class JavaRuntimeService : IJavaRuntimeService
{
    /// <summary>Default JVM install root on Debian-family images.</summary>
    public const string DefaultJvmRoot = "/usr/lib/jvm";

    /// <summary>Sentinel meaning "use PATH / JAVA_HOME".</summary>
    public const string DefaultJavaBinary = "java";

    private readonly string _jvmRoot;
    private readonly Func<string, bool> _fileExists;
    private readonly Func<string, IEnumerable<string>> _listDirectories;

    public JavaRuntimeService()
        : this(DefaultJvmRoot, File.Exists, DefaultListDirectories)
    {
    }

    // Seam for tests: the filesystem is injected so discovery can be exercised
    // without a JDK installed on the machine running the suite.
    public JavaRuntimeService(
        string jvmRoot,
        Func<string, bool> fileExists,
        Func<string, IEnumerable<string>> listDirectories)
    {
        _jvmRoot = jvmRoot;
        _fileExists = fileExists;
        _listDirectories = listDirectories;
    }

    private static IEnumerable<string> DefaultListDirectories(string root)
        => Directory.Exists(root) ? Directory.EnumerateDirectories(root) : Enumerable.Empty<string>();

    public IReadOnlyList<JavaRuntimeDto> Discover()
    {
        var runtimes = new List<JavaRuntimeDto>();

        foreach (var dir in _listDirectories(_jvmRoot))
        {
            var executable = Path.Combine(dir, "bin", "java");
            if (!_fileExists(executable))
                continue;

            var name = Path.GetFileName(dir.TrimEnd('/', '\\'));
            var major = ParseMajorVersion(name);
            runtimes.Add(new JavaRuntimeDto(executable, major, BuildLabel(name, major)));
        }

        // Newest first, then by path so the order is stable across calls.
        return runtimes
            .OrderByDescending(r => r.MajorVersion ?? -1)
            .ThenBy(r => r.Path, StringComparer.Ordinal)
            .ToList();
    }

    public string ResolveForMinecraftVersion(string? minecraftVersion)
    {
        var preferred = PreferredMajorVersions(minecraftVersion);
        if (preferred.Length == 0)
            return DefaultJavaBinary;

        var installed = Discover();
        foreach (var major in preferred)
        {
            var match = installed.FirstOrDefault(r => r.MajorVersion == major);
            if (match is not null)
                return match.Path;
        }

        return DefaultJavaBinary;
    }

    /// <summary>
    /// Feature versions to try, most preferred first, for a Minecraft version.
    /// MC 1.26+ needs Java 25, 1.21-1.25 needs 21, 1.17-1.20 needs 17 (21 works
    /// too), older needs 8. An empty result means "no opinion — use PATH".
    /// </summary>
    public static int[] PreferredMajorVersions(string? minecraftVersion)
    {
        if (string.IsNullOrWhiteSpace(minecraftVersion))
            return Array.Empty<int>();

        var parts = minecraftVersion.Split('.');
        if (!int.TryParse(parts[0], out var major))
            return Array.Empty<int>();

        // New versioning: 26.x+ (Minecraft dropped the "1." prefix).
        if (major >= 26)
            return new[] { 25, 21 };

        if (major == 1 && parts.Length >= 2 && int.TryParse(parts[1], out var minor))
        {
            if (minor >= 21) return new[] { 21 };
            if (minor >= 17) return new[] { 21, 17 };
            return new[] { 8 };
        }

        return Array.Empty<int>();
    }

    /// <summary>
    /// Extracts the feature version from a JVM directory name such as
    /// "temurin-21-jdk-amd64", "java-17-openjdk-amd64" or "zulu-8".
    /// </summary>
    public static int? ParseMajorVersion(string directoryName)
    {
        if (string.IsNullOrWhiteSpace(directoryName))
            return null;

        foreach (var segment in directoryName.Split('-', '_', '.'))
        {
            // "1.8.0" style: a leading 1 is the legacy prefix, not the version.
            if (segment is "1" or "")
                continue;
            if (int.TryParse(segment, out var value) && value is >= 6 and <= 99)
                return value;
        }

        return null;
    }

    /// <summary>Builds the picker label, e.g. "Java 21 (temurin-21-jdk-amd64)".</summary>
    public static string BuildLabel(string directoryName, int? major)
        => major is null ? directoryName : $"Java {major} ({directoryName})";
}
