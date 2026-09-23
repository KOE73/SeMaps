namespace Sample.Core;

public interface IRepository
{
    Status CurrentStatus { get; }

    event Notify Changed;

    Repo<Status> Snapshot();
}
