using System.IdentityModel.Tokens.Jwt;
using System.Linq;
using System.Security.Claims;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;
using MineOS.Application.Interfaces;
using MineOS.Domain.ValueObjects;

namespace MineOS.Api.Authorization;

public sealed class ServerAccessFilter : IEndpointFilter
{
    public async ValueTask<object?> InvokeAsync(EndpointFilterInvocationContext context, EndpointFilterDelegate next)
    {
        var httpContext = context.HttpContext;
        var user = httpContext.User;

        var hasServerName = TryGetServerName(httpContext, out var serverName);

        // Checked before anything else, administrators included. Every
        // server-scoped service builds a path with
        // Path.Combine(BaseDirectory, ServersPathSegment, serverName), so the
        // name is a path component; their GetSafePath helpers only contain the
        // *relative* part against a root this name already chose. Letting the
        // admin branch below skip the check would leave the file endpoints
        // reachable at any path on the host via a name like "../../etc".
        //
        // A non-admin is additionally stopped by the access lookup further down
        // (no grant exists for a traversal name), but that is a side effect of
        // authorisation rather than a containment guarantee, and it does not
        // apply to admins or to API-key callers at all.
        // It runs ahead of the authentication check below too: whether the name
        // is a usable path component has nothing to do with who is asking.
        if (hasServerName && !ServerName.IsPathSafe(serverName))
        {
            return Results.BadRequest(new { error = "Invalid server name" });
        }

        if (user?.Identity?.IsAuthenticated != true)
        {
            return await next(context);
        }

        var role = user.FindFirstValue(ClaimTypes.Role) ?? "user";
        if (string.Equals(role, "admin", StringComparison.OrdinalIgnoreCase))
        {
            return await next(context);
        }

        if (!TryGetUserId(user, out var userId))
        {
            return Results.Unauthorized();
        }

        if (!hasServerName)
        {
            return await next(context);
        }

        var permission = ResolvePermission(httpContext);
        var accessService = httpContext.RequestServices.GetRequiredService<IServerAccessService>();
        var access = await accessService.GetAccessAsync(userId, serverName, httpContext.RequestAborted);

        if (access == null || !HasPermission(access, permission))
        {
            return Results.Forbid();
        }

        return await next(context);
    }

    private static bool TryGetServerName(HttpContext context, out string serverName)
    {
        serverName = string.Empty;
        // Server-scoped routes use either {name} (the /servers group) or {serverName}
        // (the players/worlds groups). Check both so this filter guards all of them.
        object? value = null;
        if (context.Request.RouteValues.TryGetValue("name", out var byName) && byName != null)
        {
            value = byName;
        }
        else if (context.Request.RouteValues.TryGetValue("serverName", out var byServerName) && byServerName != null)
        {
            value = byServerName;
        }

        if (value == null)
        {
            return false;
        }

        serverName = value.ToString() ?? string.Empty;
        return !string.IsNullOrWhiteSpace(serverName);
    }

    private static bool TryGetUserId(ClaimsPrincipal user, out int userId)
    {
        userId = 0;
        var claim = user.Claims.FirstOrDefault(c => c.Type == ClaimTypes.NameIdentifier)?.Value
            ?? user.Claims.FirstOrDefault(c => c.Type == JwtRegisteredClaimNames.Sub)?.Value;
        return int.TryParse(claim, out userId);
    }

    private static ServerPermission ResolvePermission(HttpContext context)
    {
        var requirement = context.GetEndpoint()?.Metadata.GetMetadata<ServerAccessRequirement>();
        if (requirement != null)
        {
            return requirement.Permission;
        }

        var path = context.Request.Path.Value ?? string.Empty;
        if (path.Contains("/console", StringComparison.OrdinalIgnoreCase))
        {
            return ServerPermission.Console;
        }

        if (HttpMethods.IsGet(context.Request.Method) || HttpMethods.IsHead(context.Request.Method))
        {
            return ServerPermission.View;
        }

        return ServerPermission.Control;
    }

    private static bool HasPermission(Application.Dtos.ServerAccessDto access, ServerPermission permission)
    {
        return permission switch
        {
            ServerPermission.View => access.CanView || access.CanControl || access.CanConsole,
            ServerPermission.Control => access.CanControl,
            ServerPermission.Console => access.CanConsole,
            _ => false
        };
    }
}
