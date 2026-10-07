using System;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;

namespace SeMaps.VisualStudio;

/// <summary>Transport-independent session and request correlation. Called on the IDE thread.</summary>
internal sealed class IdeBridgeClient
{
    private readonly Action<JObject> send;
    private readonly Dictionary<string, TaskCompletionSource<JObject>> pending = new Dictionary<string, TaskCompletionSource<JObject>>();
    private readonly Dictionary<string, string> incoming = new Dictionary<string, string>();
    private readonly Dictionary<string, JObject> replies = new Dictionary<string, JObject>();
    public string Session { get; private set; } = Guid.NewGuid().ToString("N");
    public HashSet<string> Capabilities { get; } = new HashSet<string>(StringComparer.Ordinal);
    public Func<string, JObject, Task<JObject>>? RequestReceived { get; set; }
    public Action<string, JObject, string?>? EventReceived { get; set; }

    public IdeBridgeClient(Action<JObject> send) { this.send = send; }

    public void Reset()
    {
        foreach (var completion in pending.Values) completion.TrySetCanceled();
        pending.Clear(); incoming.Clear(); replies.Clear(); Capabilities.Clear(); Session = Guid.NewGuid().ToString("N");
    }

    public async Task<JObject> RequestAsync(string method, JObject parameters, CancellationToken token, int timeoutMs = 5000)
    {
        token.ThrowIfCancellationRequested();
        var id = Guid.NewGuid().ToString("N");
        var completion = new TaskCompletionSource<JObject>(TaskCreationOptions.RunContinuationsAsynchronously);
        var session = Session;
        pending.Add(id, completion);
        using (var timeout = CancellationTokenSource.CreateLinkedTokenSource(token))
        {
            timeout.CancelAfter(timeoutMs);
            using (timeout.Token.Register(() => completion.TrySetCanceled()))
            {
                try
                {
                    send(new JObject { ["protocol"] = "semaps.ide", ["version"] = 1, ["sessionId"] = Session,
                        ["kind"] = "request", ["id"] = id, ["method"] = method, ["params"] = parameters });
                    return await completion.Task;
                }
                catch (OperationCanceledException)
                {
                    if (Session == session && method != "bridge.initialize") send(new JObject { ["protocol"] = "semaps.ide", ["version"] = 1,
                        ["sessionId"] = Session, ["kind"] = "request", ["id"] = Guid.NewGuid().ToString("N"), ["method"] = "bridge.cancel", ["params"] = new JObject { ["requestId"] = id } });
                    throw;
                }
                finally { pending.Remove(id); }
            }
        }
    }

    public async Task ReceiveAsync(JObject message)
    {
        if (!ValidMessage(message)) return;
        if ((string?)message["protocol"] != "semaps.ide" || (int?)message["version"] != 1
            || (string?)message["sessionId"] != Session) return;
        var kind = (string?)message["kind"];
        if (kind == "response")
        {
            var id = (string?)message["requestId"];
            if (id == null || !pending.TryGetValue(id, out var completion)) return;
            if ((message["result"] is JObject) == (message["error"] is JObject)) return;
            if (message["error"] is JObject error)
                completion.TrySetException(new InvalidOperationException((string?)error["code"] + ": " + (string?)error["message"]));
            else completion.TrySetResult((JObject)message["result"]!);
        }
        else if (kind == "event" && message["params"] is JObject eventParams)
        {
            var method = (string?)message["method"];
            if (method != null) EventReceived?.Invoke(method, eventParams, (string?)message["origin"]);
        }
        else if (kind == "request" && message["params"] is JObject parameters)
        {
            var id = (string?)message["id"];
            var method = (string?)message["method"];
            if (id == null || id.Length == 0 || method == null) return;
            // Even a repeated source.open must not reopen the file twice.
            if (incoming.TryGetValue(id, out var fingerprint))
            {
                if (fingerprint != method + parameters.ToString()) send(new JObject { ["protocol"] = "semaps.ide", ["version"] = 1, ["sessionId"] = Session,
                    ["kind"] = "response", ["requestId"] = id, ["error"] = new JObject { ["code"] = "duplicateId", ["message"] = "Request id reused" } });
                else if (replies.TryGetValue(id, out var cached)) send(cached);
                return;
            }
            incoming.Add(id, method + parameters.ToString());
            var session = Session;
            var response = new JObject { ["protocol"] = "semaps.ide", ["version"] = 1,
                ["sessionId"] = session, ["kind"] = "response", ["requestId"] = id };
            try
            {
                if (method != "source.open") response["error"] = new JObject { ["code"] = "unsupported", ["message"] = method };
                else response["result"] = RequestReceived == null ? new JObject { ["opened"] = false } : await RequestReceived(method, parameters);
            }
            catch (Exception ex) { response["error"] = new JObject { ["code"] = "failed", ["message"] = ex.Message }; }
            if (Session == session)
            {
                replies[id] = response;
                if (replies.Count > 200) { var first = new List<string>(replies.Keys)[0]; incoming.Remove(first); replies.Remove(first); }
                send(response);
            }
        }
    }

    internal static bool ValidMessage(JObject message)
    {
        var allowed = new HashSet<string>(new[] { "protocol", "version", "sessionId", "kind", "id", "requestId", "method", "params", "result", "error", "origin", "causationId" });
        if (message.Properties().Any(p => !allowed.Contains(p.Name)) || message["protocol"]?.Type != JTokenType.String || (string?)message["protocol"] != "semaps.ide"
            || message["version"]?.Type != JTokenType.Integer || message["version"]!.ToString() != "1" || !StringField(message["sessionId"])) return false;
        if (new[] { "origin", "causationId" }.Any(key => message[key] != null && !StringField(message[key]))) return false;
        var kind = message["kind"]?.Type == JTokenType.String ? (string?)message["kind"] : null;
        if (kind == "response")
        {
            if (!StringField(message["requestId"]) || new[] { "id", "method", "params" }.Any(key => message[key] != null)) return false;
            if ((message["result"] != null) == (message["error"] != null)) return false;
            if (message["result"] != null) return message["result"] is JObject;
            return message["error"] is JObject error && StringField(error["code"]) && error["message"]?.Type == JTokenType.String
                && error.Properties().All(p => p.Name == "code" || p.Name == "message" || p.Name == "data");
        }
        return (kind == "request" || kind == "event") && StringField(message["id"]) && StringField(message["method"])
            && System.Text.RegularExpressions.Regex.IsMatch((string)message["method"]!, "^[a-z][a-zA-Z]*\\.[a-z][a-zA-Z]*$")
            && (message["params"] == null || message["params"] is JObject)
            && !new[] { "requestId", "result", "error" }.Any(key => message[key] != null);
    }

    private static bool StringField(JToken? value) => value?.Type == JTokenType.String && !string.IsNullOrEmpty(value.Value<string>());
}
