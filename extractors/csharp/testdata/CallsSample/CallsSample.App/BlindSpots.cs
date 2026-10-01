using System;
using System.Reflection;

namespace CallsSample.App;

// One method per mark kind of ADR_20260928-5 §4, plus a method with two marks and one mark
// inside a lambda (attributed to the enclosing method, like a call: EXTRACTOR.md §2.3).
public class BlindSpots
{
    public object CreateViaActivator(Type t)
    {
        return Activator.CreateInstance(t)!;
    }

    public object? InvokeViaReflection(MethodInfo method, object target)
    {
        return method.Invoke(target, null);
    }

    public Type BuildGenericType(Type open, Type argument)
    {
        return open.MakeGenericType(argument);
    }

    public void DynamicCallInLambda(dynamic value)
    {
        Action act = () => value.DoWork();
        act();
    }

    public object CreateTwice(Type t)
    {
        var a = Activator.CreateInstance(t);
        var b = Activator.CreateInstance(t, 1)!;
        return a ?? b;
    }
}
