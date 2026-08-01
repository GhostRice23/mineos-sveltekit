using System.Security.Cryptography;
using System.Text;

namespace MineOS.Infrastructure.Services;

/// <summary>
/// Turns an API key into the value stored in the database.
///
/// SHA-256 rather than Argon2 (which <see cref="Argon2PasswordHasher"/> uses for
/// passwords) on purpose. A password is low-entropy and human-chosen, so it
/// needs a deliberately slow KDF to make guessing expensive. An API key here is
/// 256 bits from <see cref="RandomNumberGenerator"/>: there is nothing to guess,
/// so the only thing a slow hash would buy is latency on every single
/// authenticated request, and no salt is needed because the input is already
/// unique and unguessable.
///
/// Being unsalted is also what makes the lookup work: the database is queried by
/// hash, so the same key has to produce the same value every time.
/// </summary>
public static class ApiKeyHasher
{
    /// <summary>Lowercase hex of the SHA-256 of the UTF-8 bytes of <paramref name="apiKey"/>.</summary>
    public static string Hash(string apiKey)
    {
        ArgumentNullException.ThrowIfNull(apiKey);
        var digest = SHA256.HashData(Encoding.UTF8.GetBytes(apiKey));
        return Convert.ToHexString(digest).ToLowerInvariant();
    }

    /// <summary>
    /// Whether <paramref name="candidate"/> looks like a value this class
    /// produced. Used to tell an already-upgraded row from a legacy plaintext
    /// one without keeping a separate flag column.
    /// </summary>
    public static bool LooksLikeHash(string? candidate)
    {
        if (candidate is not { Length: 64 })
        {
            return false;
        }

        foreach (var c in candidate)
        {
            var isHexDigit = c is >= '0' and <= '9' or >= 'a' and <= 'f';
            if (!isHexDigit)
            {
                return false;
            }
        }

        return true;
    }
}
