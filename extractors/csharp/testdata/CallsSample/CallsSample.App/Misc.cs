using System;

namespace CallsSample.App;

// A lambda's call belongs to the enclosing method, not to the lambda (which
// is never a symbol of its own: EXTRACTOR.md §2.3).
public class LambdaHost
{
    public void Run()
    {
        Action act = () => Helper();
        act();
    }

    private void Helper()
    {
    }
}

// Same for a local function's call.
public class LocalFnHost
{
    public void Run()
    {
        void Local() => Helper();
        Local();
    }

    private void Helper()
    {
    }
}

public class PropertyHost
{
    public int Value { get; set; }

    public void Touch()
    {
        var current = Value; // read
        Value = current + 1; // write
    }
}

// A constructor chain: ChainCtor() calls ChainCtor(int) via `this(...)`.
public class ChainCtor
{
    public ChainCtor()
        : this(0)
    {
    }

    public ChainCtor(int seed)
    {
        Seed = seed;
    }

    public int Seed;
}
