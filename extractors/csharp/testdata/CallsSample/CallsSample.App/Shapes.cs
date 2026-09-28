namespace CallsSample.App;

public interface IShape
{
    double Area();
}

public class Circle : IShape
{
    public double Radius;

    public double Area() => Radius * Radius * 3.14159;
}

public class Square : IShape
{
    public double Side;

    public double Area() => Side * Side;
}

// Explicit interface implementation: same method symbol as any other, with an
// `implements` edge to IShape.Area (docs/EXTRACTOR.md §3).
public class Robot : IShape
{
    double IShape.Area() => 0;
}

public class ShapeUser
{
    // A call through an interface: the target as written at the call site is
    // IShape.Area, not whichever implementer actually runs (ADR_20260928-4).
    public double Measure(IShape shape) => shape.Area();
}

public static class Factory
{
    // The case that had zero edges before this ADR: a static factory that
    // constructs an object and returns it — `constructs` to Circle, plus
    // `calls` to its constructor when the constructor is a symbol of this
    // output (here the implicit one, since Circle declares none explicitly).
    public static IShape MakeCircle(double radius) => new Circle { Radius = radius };
}
