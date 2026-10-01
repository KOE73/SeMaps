namespace CallsSample.App;

// Mutual recursion across two types: MutualA.Ping calls MutualB.Pong, which
// constructs a new MutualA and calls its Ping.
public class MutualA
{
    public void Ping(int n)
    {
        if (n > 0)
        {
            MutualB.Pong(n - 1);
        }
    }
}

public static class MutualB
{
    public static void Pong(int n)
    {
        if (n > 0)
        {
            new MutualA().Ping(n - 1);
        }
    }
}
