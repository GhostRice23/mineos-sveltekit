using MineOS.Infrastructure.Services;

namespace MineOS.Tests.Unit;

public class ApiKeyHasherTests
{
    [Fact]
    public void Hashing_is_deterministic_and_hex()
    {
        var first = ApiKeyHasher.Hash("s3cret");
        var second = ApiKeyHasher.Hash("s3cret");

        Assert.Equal(first, second);
        Assert.Equal(64, first.Length);
        Assert.True(ApiKeyHasher.LooksLikeHash(first));
    }

    [Fact]
    public void Known_vector()
    {
        // SHA-256("abc"), so a future change to the algorithm or the encoding
        // shows up here rather than as keys that silently stop working.
        Assert.Equal(
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
            ApiKeyHasher.Hash("abc"));
    }

    [Fact]
    public void Different_keys_hash_differently()
    {
        Assert.NotEqual(ApiKeyHasher.Hash("key-a"), ApiKeyHasher.Hash("key-b"));
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("too-short")]
    [InlineData("BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD")]
    public void LooksLikeHash_rejects_anything_else(string? candidate)
    {
        // Uppercase included on purpose: Hash only ever emits lowercase, so an
        // uppercase string did not come from here and must not be mistaken for
        // an already-upgraded value.
        Assert.False(ApiKeyHasher.LooksLikeHash(candidate));
    }
}
