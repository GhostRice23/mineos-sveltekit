using System.Security.Cryptography;
using System.Text;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;
using MineOS.Application.Interfaces;
using MineOS.Application.Options;
using MineOS.Infrastructure.Persistence;

namespace MineOS.Infrastructure.Services;

public sealed class ApiKeyValidator : IApiKeyValidator
{
    private readonly AppDbContext _db;
    private readonly ApiKeyOptions _options;

    public ApiKeyValidator(AppDbContext db, IOptions<ApiKeyOptions> options)
    {
        _db = db;
        _options = options.Value;
    }

    public Task<bool> IsValidAsync(string apiKey, CancellationToken cancellationToken)
    {
        if (!string.IsNullOrWhiteSpace(_options.StaticKey) &&
            FixedTimeEquals(_options.StaticKey, apiKey))
        {
            return Task.FromResult(true);
        }

        return _db.ApiKeys.AnyAsync(k => !k.Revoked && k.Key == apiKey, cancellationToken);
    }

    /// <summary>
    /// Compares two secrets without leaking their common prefix length through
    /// timing. A valid API key carries admin identity, so an ordinary
    /// <c>string.Equals</c> here lets an attacker recover the configured key one
    /// character at a time by measuring responses.
    /// </summary>
    private static bool FixedTimeEquals(string expected, string? actual)
    {
        if (actual == null)
        {
            return false;
        }

        var expectedBytes = Encoding.UTF8.GetBytes(expected);
        var actualBytes = Encoding.UTF8.GetBytes(actual);

        // FixedTimeEquals is only constant-time for equal-length inputs; it
        // returns false immediately otherwise. Length is not the secret here —
        // the key's contents are — so that is the accepted, standard tradeoff.
        return CryptographicOperations.FixedTimeEquals(expectedBytes, actualBytes);
    }
}
