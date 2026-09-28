namespace CallsSample.App;

public abstract class Animal
{
    public virtual string Speak() => "...";
}

// First link of an override chain: Dog.Speak overrides Animal.Speak, and calls
// it through `base.Speak()`.
public class Dog : Animal
{
    public override string Speak() => base.Speak() + "Woof";
}

// Second link: Puppy.Speak overrides Dog.Speak (not Animal.Speak directly).
public class Puppy : Dog
{
    public override string Speak() => base.Speak() + "!";
}
