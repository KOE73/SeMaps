namespace CallsSample.App;

public class Calculator
{
    // Overloads: distinct ids (different parameter lists), and a call that
    // picks a specific overload by argument count.
    public int Add(int a, int b) => a + b;

    public int Add(int a, int b, int c) => Add(Add(a, b), c);

    // A generic method, called with two different type arguments: both
    // calls resolve to the same original definition (ADR_20260928-4 §1).
    public T Identity<T>(T value) => value;

    public void UseIdentity()
    {
        var text = Identity("a");
        var number = Identity(1);
    }
}

public class Recursive
{
    // Recursion: a method calling itself is a self-loop edge, allowed only
    // for `calls` (core/facts.go).
    public int Fib(int n) => n <= 1 ? n : Fib(n - 1) + Fib(n - 2);
}
