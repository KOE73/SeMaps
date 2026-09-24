using System;
using System.Collections.Generic;

namespace Sample.Core;

/// <summary>
/// Exercises member relations features: List<T>, IReadOnlyList<T>, Dictionary<K,V>,
/// T?, Task<T>, T[], Func<T>, Lazy<T>, constructor parameters, and unknown generics.
/// </summary>
public class Features
{
    // List<T> with mutable many
    public List<Status> Statuses { get; set; } = [];

    // IReadOnlyList<T> with readonly many
    public IReadOnlyList<Status> ReadOnlyStatuses { get; set; } = [];

    // Dictionary<K,V> with symbol key and value (generates two edges)
    public Dictionary<Status, Repo<Status>> StatusMap { get; set; } = [];

    // T[] array with item path
    public Status[] StatusArray { get; set; } = [];

    // Nullable<T> with optional cardinality
    public Status? OptionalStatus { get; set; }

    // Task<T> with deferred flag and result path
    public Task<Status> GetStatusAsync() => Task.FromResult(Status.Draft);

    // Func<T> with deferred flag (delegate returning T)
    public Func<Status> StatusFactory { get; set; } = () => Status.Draft;

    // Lazy<T> with deferred flag
    public Lazy<Status> LazyStatus { get; set; } = new(() => Status.Draft);

    // Constructor with parameter (uses edge with parameter name)
    public Features(Status initialStatus)
    {
        OptionalStatus = initialStatus;
    }

    // Method with unknown generic type argument
    public void ProcessGeneric<T>(T item) where T : class
    {
    }
}
