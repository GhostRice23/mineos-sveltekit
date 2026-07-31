using System.Net;

namespace MineOS.Tests.Integration;

/// <summary>
/// ServerNameTests pins the policy; these pin the wiring — that the check sits
/// in the request pipeline and, in particular, that the administrator branch of
/// ServerAccessFilter does not skip past it.
///
/// The client here carries the static X-Api-Key, which is admin identity. That
/// is the case worth testing: a non-admin is stopped anyway by the access
/// lookup (no grant exists for a name no server has), so the admin path is the
/// one where containment has to be its own guarantee.
///
/// The names below traverse nothing on their own. They are chosen because they
/// survive URL routing as a single unmodified segment, so the assertion is
/// about the filter rather than about how a server normalises "%2E%2E%2F".
/// </summary>
public class ServerNameGuardTests : IClassFixture<MineOsWebApplicationFactory>
{
    private readonly HttpClient _client;

    public ServerNameGuardTests(MineOsWebApplicationFactory factory)
    {
        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Add("X-Api-Key", "dev-static-api-key-change-me");
    }

    [Theory]
    [InlineData("sur..vival")]
    [InlineData("has:colon")]
    public async Task Admin_cannot_reach_a_server_route_with_an_unsafe_name(string name)
    {
        var response = await _client.GetAsync($"/api/v1/servers/{name}/files");

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
    }

    [Fact]
    public async Task An_ordinary_name_is_not_blocked_by_the_guard()
    {
        // No such server exists, so this must fail for a reason that is not the
        // name check -- otherwise the guard could be rejecting everything and
        // the test above would still pass.
        var response = await _client.GetAsync("/api/v1/servers/survival/files");

        Assert.NotEqual(HttpStatusCode.BadRequest, response.StatusCode);
    }
}
