using System.Collections.Generic;

namespace Sample.Core;

/// <summary>
/// Exercises member relations features: List<T>, IReadOnlyList<T>, Dictionary<K,V>,
/// T?, Task<T>, constructor parameters, and unknown generics.
/// </summary>
public class Features
{
    // List<T> with mutable many
    public List<Status> Statuses { get; set; } = [];

    // IReadOnlyList<T> with readonly many
    public IReadOnlyList<Status> ReadOnlyStatuses { get; set; } = [];

    // Dictionary<K,V> with symbol key and value (generates two edges)
    public Dictionary<Status, Repo<Status>> StatusMap { get; set; } = [];

    // Nullable<T> with optional cardinality
    public Status? OptionalStatus { get; set; }

    // Task<T> with deferred flag
    public Task<Status> GetStatusAsync() => Task.FromResult(Status.Draft);

    // Constructor with parameter (uses edge)
    public Features(Status initialStatus)
    {
        OptionalStatus = initialStatus;
    }

    // Method with unknown generic type argument
    public void ProcessGeneric<T>(T item) where T : class
    {
    }
}
