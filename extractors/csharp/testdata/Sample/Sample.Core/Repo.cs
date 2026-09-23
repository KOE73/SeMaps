namespace Sample.Core;

public class Repo<T>
{
    public T? Current { get; set; }

    public List<T> Items { get; } = [];
}
