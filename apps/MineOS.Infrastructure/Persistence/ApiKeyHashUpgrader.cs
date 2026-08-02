using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using MineOS.Infrastructure.Services;

namespace MineOS.Infrastructure.Persistence;

/// <summary>
/// Converts rows written before API keys were hashed. Runs once per startup,
/// right after migrations and before <see cref="ApiKeySeeder"/>, so the seeder's
/// "are there already keys?" check sees upgraded rows.
///
/// This is the data half of the change, and it cannot live in the migration
/// itself: SQLite has no SHA-256, and an EF migration can only emit SQL. So the
/// migration adds the column and this fills it.
///
/// Existing keys keep working — the value is unchanged, only how it is stored.
/// </summary>
public sealed class ApiKeyHashUpgrader
{
    private readonly AppDbContext _db;
    private readonly ILogger<ApiKeyHashUpgrader> _logger;

    public ApiKeyHashUpgrader(AppDbContext db, ILogger<ApiKeyHashUpgrader> logger)
    {
        _db = db;
        _logger = logger;
    }

    public async Task UpgradeAsync(CancellationToken cancellationToken)
    {
        var legacy = await _db.ApiKeys
            .Where(k => k.KeyHash == null && k.Key != null)
            .ToListAsync(cancellationToken);

        if (legacy.Count == 0)
        {
            return;
        }

        var upgraded = 0;
        foreach (var row in legacy)
        {
            var plaintext = row.Key;
            if (string.IsNullOrWhiteSpace(plaintext))
            {
                continue;
            }

            // A row could already hold a hash if an upgrade was interrupted
            // between writing KeyHash and clearing Key. Re-hashing it would
            // silently invalidate a working key, so detect and just clear.
            row.KeyHash = ApiKeyHasher.LooksLikeHash(plaintext)
                ? plaintext
                : ApiKeyHasher.Hash(plaintext.Trim());

            // Clearing the plaintext is the point of the exercise. The column
            // itself is dropped in a later migration, once no deployment can
            // still be mid-upgrade.
            row.Key = null;
            upgraded++;
        }

        if (upgraded == 0)
        {
            return;
        }

        await _db.SaveChangesAsync(cancellationToken);
        _logger.LogInformation(
            "Upgraded {Count} stored API key(s) to hashed storage. The keys themselves are unchanged.",
            upgraded);
    }
}
