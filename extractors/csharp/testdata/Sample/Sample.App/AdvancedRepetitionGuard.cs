using Sample.Core;
using Sample.Core.Guards;

namespace Sample.App;

public class AdvancedRepetitionGuard : RepetitionGuard, IRepository
{
    public Status CurrentStatus { get; set; }

    public event Notify? Changed;

    public Repo<Status> Snapshot() => new();
}
