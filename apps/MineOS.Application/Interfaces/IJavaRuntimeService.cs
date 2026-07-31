namespace MineOS.Application.Interfaces;

/// <summary>
/// A Java runtime installed on the host.
/// </summary>
/// <param name="Path">Absolute path to the java executable.</param>
/// <param name="MajorVersion">Feature version (8, 17, 21, 25), or null if it could not be determined.</param>
/// <param name="Label">Human-readable name for a picker, e.g. "Java 21 (Temurin)".</param>
public record JavaRuntimeDto(string Path, int? MajorVersion, string Label);

/// <summary>
/// Discovers the Java runtimes actually present on the host.
/// </summary>
/// <remarks>
/// The web UI used to offer a hardcoded list of guessed paths that did not even
/// match the ones the server launcher probes, so a user could pick a runtime
/// that does not exist in the container and only find out when the server
/// refused to start.
/// </remarks>
public interface IJavaRuntimeService
{
    /// <summary>
    /// Lists installed runtimes, newest feature version first.
    /// </summary>
    IReadOnlyList<JavaRuntimeDto> Discover();

    /// <summary>
    /// Returns the java executable to use for a Minecraft version, or "java"
    /// to defer to PATH/JAVA_HOME.
    /// </summary>
    string ResolveForMinecraftVersion(string? minecraftVersion);
}
