using System;
using System.Text.RegularExpressions;

namespace MineOS.Domain.ValueObjects;

/// <summary>
/// Rules for a server name, which is not just a label: every server-scoped
/// service turns it into a filesystem path with
/// <c>Path.Combine(BaseDirectory, ServersPathSegment, serverName)</c>.
///
/// That makes the name a path component, and an unchecked one escapes the
/// servers directory. The per-service <c>GetSafePath</c> helpers do not help
/// here — they normalise the <em>relative</em> part against a root that the
/// name itself already determined, so a name like <c>../../etc</c> moves the
/// root and the containment check then passes against the moved root.
///
/// Two levels are deliberate:
///
/// <list type="bullet">
/// <item><see cref="IsWellFormed"/> is the strict allowlist applied when a
/// server is created. New names should be boring.</item>
/// <item><see cref="IsPathSafe"/> is the weaker check applied to every request
/// that names an existing server. It rejects everything that can traverse or
/// otherwise escape, but tolerates names that predate the allowlist or were
/// created outside the API — refusing those would lock an operator out of a
/// server that exists and is running.</item>
/// </list>
///
/// Anything <see cref="IsWellFormed"/> accepts, <see cref="IsPathSafe"/>
/// accepts too; ServerNameTests pins that relationship.
/// </summary>
public static class ServerName
{
    /// <summary>Longest accepted name, chosen to stay well inside NAME_MAX.</summary>
    public const int MaxLength = 64;

    private static readonly Regex WellFormedPattern = new(
        @"^[a-zA-Z0-9][a-zA-Z0-9 _\-\.]{0,63}$",
        RegexOptions.Compiled | RegexOptions.CultureInvariant);

    /// <summary>
    /// The strict allowlist for newly created servers.
    /// </summary>
    public static bool IsWellFormed(string? name)
    {
        if (string.IsNullOrWhiteSpace(name))
        {
            return false;
        }

        return WellFormedPattern.IsMatch(name) && IsPathSafe(name);
    }

    /// <summary>
    /// Whether the name is safe to use as a single path component. This is the
    /// check that belongs on every request, including an administrator's.
    /// </summary>
    public static bool IsPathSafe(string? name)
    {
        if (string.IsNullOrWhiteSpace(name) || name.Length > MaxLength)
        {
            return false;
        }

        // "." and ".." are the traversal primitives; any name containing a ".."
        // run is rejected outright rather than reasoned about.
        if (name == "." || name == ".." || name.Contains("..", StringComparison.Ordinal))
        {
            return false;
        }

        foreach (var c in name)
        {
            // Both separators, regardless of host OS: the check must not depend
            // on which platform happens to be running it.
            if (c == '/' || c == '\\')
            {
                return false;
            }

            // A colon would let a Windows drive-relative path ("C:foo") or an
            // NTFS alternate data stream through.
            if (c == ':')
            {
                return false;
            }

            // Control characters, including the NUL that truncates a path in
            // any native API the runtime hands it to.
            if (char.IsControl(c))
            {
                return false;
            }
        }

        // A leading or trailing space or dot is how a name ends up resolving to
        // its parent, or to a different file than it appears to name.
        if (name[0] == ' ' || name[0] == '.' ||
            name[^1] == ' ' || name[^1] == '.')
        {
            return false;
        }

        return true;
    }
}
