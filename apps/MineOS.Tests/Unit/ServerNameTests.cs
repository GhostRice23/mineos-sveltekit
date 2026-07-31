using MineOS.Domain.ValueObjects;

namespace MineOS.Tests.Unit;

/// <summary>
/// A server name becomes a path component in every server-scoped service, so
/// these are containment tests rather than input-cosmetics tests. The traversal
/// cases below are the ones ServerAccessFilter relies on rejecting.
/// </summary>
public class ServerNameTests
{
    [Theory]
    [InlineData("survival")]
    [InlineData("Creative 2")]
    [InlineData("my-server_1.20.4")]
    [InlineData("a")]
    public void Ordinary_names_are_well_formed(string name)
    {
        Assert.True(ServerName.IsWellFormed(name));
        Assert.True(ServerName.IsPathSafe(name));
    }

    [Theory]
    [InlineData("../etc")]
    [InlineData("../../etc/passwd")]
    [InlineData("..")]
    [InlineData(".")]
    [InlineData("..\\..\\windows")]
    [InlineData("foo/bar")]
    [InlineData("foo\\bar")]
    [InlineData("/absolute")]
    [InlineData("C:windows")]
    [InlineData("stream:name")]
    public void Traversal_shapes_are_rejected(string name)
    {
        Assert.False(ServerName.IsPathSafe(name));
        Assert.False(ServerName.IsWellFormed(name));
    }

    [Fact]
    public void A_dot_dot_run_is_rejected_even_mid_name()
    {
        // Not reachable as traversal on its own, but nothing legitimate needs it
        // and allowing it means reasoning about every consumer's normalisation.
        Assert.False(ServerName.IsPathSafe("sur..vival"));
    }

    [Fact]
    public void Embedded_nul_is_rejected()
    {
        // A NUL truncates the path in the native APIs the runtime forwards to,
        // so "survival\0.png" can name a different file than it appears to.
        Assert.False(ServerName.IsPathSafe("survival\0.png"));
    }

    [Theory]
    [InlineData("\r\n")]
    [InlineData("bad\tname")]
    [InlineData("bad\nname")]
    public void Control_characters_are_rejected(string name)
    {
        Assert.False(ServerName.IsPathSafe(name));
    }

    [Theory]
    [InlineData(" leading")]
    [InlineData("trailing ")]
    [InlineData(".hidden")]
    [InlineData("trailing.")]
    public void Leading_and_trailing_dots_and_spaces_are_rejected(string name)
    {
        Assert.False(ServerName.IsPathSafe(name));
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    public void Empty_names_are_rejected(string? name)
    {
        Assert.False(ServerName.IsPathSafe(name));
        Assert.False(ServerName.IsWellFormed(name));
    }

    [Fact]
    public void Names_longer_than_the_limit_are_rejected()
    {
        Assert.True(ServerName.IsPathSafe(new string('a', ServerName.MaxLength)));
        Assert.False(ServerName.IsPathSafe(new string('a', ServerName.MaxLength + 1)));
    }

    [Fact]
    public void The_creation_allowlist_is_stricter_than_the_path_check()
    {
        // Unicode is a fine directory name, so it stays usable for a server that
        // already exists -- but new servers are held to the boring allowlist.
        Assert.True(ServerName.IsPathSafe("überleben"));
        Assert.False(ServerName.IsWellFormed("überleben"));
    }

    [Fact]
    public void Anything_well_formed_is_also_path_safe()
    {
        // The relationship the filter depends on: tightening creation can never
        // lock an operator out of a server that creation itself accepted.
        string[] candidates =
        [
            "survival", "Creative 2", "my-server_1.20.4", "a", "A.B-C_D",
            "..", ".hidden", "foo/bar", "trailing.", " leading", "überleben"
        ];

        foreach (var candidate in candidates)
        {
            if (ServerName.IsWellFormed(candidate))
            {
                Assert.True(
                    ServerName.IsPathSafe(candidate),
                    $"'{candidate}' passed the creation allowlist but not the path check");
            }
        }
    }
}
