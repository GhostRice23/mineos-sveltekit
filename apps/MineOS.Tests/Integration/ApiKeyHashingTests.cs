using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using MineOS.Application.Options;
using MineOS.Domain.Entities;
using MineOS.Infrastructure.Persistence;
using MineOS.Infrastructure.Services;
using Moq;

namespace MineOS.Tests.Integration;

/// <summary>
/// API keys used to be stored as the key itself, so a database dump handed over
/// working admin credentials. They are stored as a SHA-256 hash now.
///
/// Runs against real migrations on in-memory SQLite (same shape as
/// DatabaseSchemaTests) rather than the InMemory provider, so the schema change
/// that makes this possible is exercised here too.
/// </summary>
public class ApiKeyHashingTests : IDisposable
{
    private readonly SqliteConnection _connection;
    private readonly AppDbContext _context;

    public ApiKeyHashingTests()
    {
        _connection = new SqliteConnection("DataSource=:memory:");
        _connection.Open();

        _context = new AppDbContext(new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(_connection)
            .Options);
        _context.Database.Migrate();
    }

    private ApiKeyValidator Validator() =>
        new(_context, Options.Create(new ApiKeyOptions()));

    private ApiKeyHashUpgrader Upgrader() =>
        new(_context, Mock.Of<ILogger<ApiKeyHashUpgrader>>());

    private static ApiKey Row(string name = "default") => new()
    {
        UserId = 1,
        Name = name,
        Permissions = """["*"]""",
        CreatedAt = DateTimeOffset.UtcNow,
        Revoked = false
    };

    [Fact]
    public async Task A_hashed_key_authenticates()
    {
        const string key = "s3cret-api-key";
        var row = Row();
        row.KeyHash = ApiKeyHasher.Hash(key);
        _context.ApiKeys.Add(row);
        await _context.SaveChangesAsync();

        Assert.True(await Validator().IsValidAsync(key, CancellationToken.None));
    }

    [Fact]
    public async Task The_stored_value_is_not_the_key()
    {
        const string key = "s3cret-api-key";
        var row = Row();
        row.KeyHash = ApiKeyHasher.Hash(key);
        _context.ApiKeys.Add(row);
        await _context.SaveChangesAsync();

        var stored = await _context.ApiKeys.AsNoTracking().SingleAsync();
        Assert.Null(stored.Key);
        Assert.NotEqual(key, stored.KeyHash);
        // Presenting what the database holds must not authenticate -- that is
        // the whole point of hashing it.
        Assert.False(await Validator().IsValidAsync(stored.KeyHash!, CancellationToken.None));
    }

    [Fact]
    public async Task A_wrong_or_revoked_key_is_rejected()
    {
        const string key = "s3cret-api-key";
        var row = Row();
        row.KeyHash = ApiKeyHasher.Hash(key);
        _context.ApiKeys.Add(row);
        await _context.SaveChangesAsync();

        Assert.False(await Validator().IsValidAsync("not-the-key", CancellationToken.None));

        row.Revoked = true;
        await _context.SaveChangesAsync();
        Assert.False(await Validator().IsValidAsync(key, CancellationToken.None));
    }

    [Fact]
    public async Task A_legacy_plaintext_row_is_upgraded_and_the_key_keeps_working()
    {
        const string key = "key-from-before-the-change";
        var row = Row();
        row.Key = key;
        _context.ApiKeys.Add(row);
        await _context.SaveChangesAsync();

        // Before the upgrade the key does not authenticate, because nothing
        // reads the plaintext column any more.
        Assert.False(await Validator().IsValidAsync(key, CancellationToken.None));

        await Upgrader().UpgradeAsync(CancellationToken.None);

        var stored = await _context.ApiKeys.AsNoTracking().SingleAsync();
        Assert.Null(stored.Key);
        Assert.Equal(ApiKeyHasher.Hash(key), stored.KeyHash);
        // The operator's key is unchanged -- only how it is stored.
        Assert.True(await Validator().IsValidAsync(key, CancellationToken.None));
    }

    [Fact]
    public async Task Upgrading_twice_changes_nothing()
    {
        const string key = "key-from-before-the-change";
        var row = Row();
        row.Key = key;
        _context.ApiKeys.Add(row);
        await _context.SaveChangesAsync();

        await Upgrader().UpgradeAsync(CancellationToken.None);
        var afterFirst = (await _context.ApiKeys.AsNoTracking().SingleAsync()).KeyHash;
        await Upgrader().UpgradeAsync(CancellationToken.None);
        var afterSecond = (await _context.ApiKeys.AsNoTracking().SingleAsync()).KeyHash;

        Assert.Equal(afterFirst, afterSecond);
        Assert.True(await Validator().IsValidAsync(key, CancellationToken.None));
    }

    [Fact]
    public async Task An_interrupted_upgrade_does_not_double_hash()
    {
        // KeyHash written, Key not yet cleared: re-hashing the already-hashed
        // value would quietly invalidate a working key.
        const string key = "s3cret-api-key";
        var hash = ApiKeyHasher.Hash(key);
        var row = Row();
        row.Key = hash;
        _context.ApiKeys.Add(row);
        await _context.SaveChangesAsync();

        await Upgrader().UpgradeAsync(CancellationToken.None);

        var stored = await _context.ApiKeys.AsNoTracking().SingleAsync();
        Assert.Null(stored.Key);
        Assert.Equal(hash, stored.KeyHash);
        Assert.True(await Validator().IsValidAsync(key, CancellationToken.None));
    }

    public void Dispose()
    {
        _context.Dispose();
        _connection.Dispose();
        GC.SuppressFinalize(this);
    }
}
