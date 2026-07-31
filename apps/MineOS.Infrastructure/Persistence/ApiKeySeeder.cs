using System.Security.Cryptography;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using MineOS.Domain.Entities;

namespace MineOS.Infrastructure.Persistence;

public sealed class ApiKeySeeder
{
    private readonly AppDbContext _db;
    private readonly IConfiguration _config;
    private readonly ILogger<ApiKeySeeder> _logger;

    public ApiKeySeeder(AppDbContext db, IConfiguration config, ILogger<ApiKeySeeder> logger)
    {
        _db = db;
        _config = config;
        _logger = logger;
    }

    public async Task EnsureSeedAsync(CancellationToken cancellationToken)
    {
        var staticKey = _config["ApiKey:StaticKey"];
        if (!string.IsNullOrWhiteSpace(staticKey))
        {
            _logger.LogInformation("Static API key configured; skipping API key seed.");
            return;
        }

        if (await _db.ApiKeys.AnyAsync(cancellationToken))
        {
            return;
        }

        // Written as two branches rather than a bool + reassignment so that
        // seedKey is provably non-null afterwards: the compiler narrows on
        // string.IsNullOrWhiteSpace directly ([NotNullWhen(false)]), but not on
        // a bool holding its result.
        var configuredKey = _config["ApiKey:SeedKey"];
        string seedKey;
        bool wasGenerated;
        if (string.IsNullOrWhiteSpace(configuredKey))
        {
            seedKey = Convert.ToBase64String(RandomNumberGenerator.GetBytes(32));
            wasGenerated = true;
        }
        else
        {
            seedKey = configuredKey;
            wasGenerated = false;
        }

        var apiKey = new ApiKey
        {
            UserId = 1, // Will be associated with first user
            Key = seedKey.Trim(),
            Name = "default",
            Permissions = """["*"]""", // Full permissions
            CreatedAt = DateTimeOffset.UtcNow,
            Revoked = false
        };

        _db.ApiKeys.Add(apiKey);
        await _db.SaveChangesAsync(cancellationToken);

        // This key carries ["*"], and a valid API key is admin identity, so the
        // value is not written to the log in the normal case: the operator
        // supplied ApiKey:SeedKey (the CLI installer puts it in .env) and
        // already has it. Logging it again only copies an admin credential into
        // wherever logs are shipped and retained.
        //
        // The generated fallback is the exception. Nothing else ever displays
        // that value, so withholding it would leave an unusable install; it is
        // logged once, as a warning, saying so.
        if (wasGenerated)
        {
            _logger.LogWarning(
                "No ApiKey:SeedKey was configured, so one was generated: {ApiKey}. " +
                "This is the only time it is shown. Store it somewhere safe, and " +
                "treat this log entry as a secret until you rotate the key.",
                apiKey.Key);
        }
        else
        {
            _logger.LogInformation("Seeded the configured API key.");
        }
    }
}
