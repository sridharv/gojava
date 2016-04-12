package go;

import go.LoadJNI;
import java.util.Arrays;
import java.lang.Math;
import java.lang.reflect.Method;
import java.util.regex.Pattern;

import go.SeqTest;

public class MoreAsserts {
    public static void assertTrue(String msg, boolean condition) {
        if (!condition) {
            throw new RuntimeException(msg);
        }
    }

    public static void assertTrue(boolean condition) {
        if (!condition) {
            throw new RuntimeException("assert failed");
        }
    }

    public static void assertEquals(int expected, int actual) {
        assertTrue(expected == actual);
    }

    public static void assertFalse(boolean condition) {
        assertTrue(!condition);
    }

    public static void assertFalse(String msg, boolean condition) {
        assertTrue(msg, !condition);
    }

    public static void assertEquals(String msg, int expected, int actual) {
        assertTrue(msg, expected == actual);
    }

    public static void assertEquals(String msg, long expected, long actual) {
        assertTrue(msg, expected == actual);
    }

    public static void assertEquals(String msg, String expected, String actual) {
        assertTrue(String.format("%s expected:%s != actual:%s", msg, expected, actual), expected.equals(actual));
    }

    public static void assertEquals(String msg, boolean expected, boolean actual) {
        assertTrue(msg, expected == actual);
    }
    public static void assertEquals(String msg, byte[] expected, byte[] actual) {
        assertTrue(msg, Arrays.equals(expected, actual));
    }

    public static void assertEquals(String msg, double expected, double actual, double epsilon) {
        assertTrue(msg, Math.abs(expected - actual) < epsilon);
    }

    public static void assertEquals(String msg, Object expected, Object actual) {
        assertTrue(msg, (expected == null && actual == null) || (expected.equals(actual)));
    }

    public static void fail(String msg) {
        throw new RuntimeException(msg);
    }

    private static boolean failed = false;

    private static String pattern = ".*";

    public static void main(String[] args) {
        if (args.length > 0) {
            pattern = args[0];
        }

        new TestRunner(new SeqTest()).runTests();
        new TestRunner(new ExtraTest()).runTests();

        // NOTE: We need to call System.exit to force all go threads to exit.
        System.exit(failed ? 1 : 0);
    }

    private static class TestRunner<T> {
        private T test;

        public TestRunner(T test) {
            this.test = test;
        }

        public void runTests() {
            Class c = test.getClass();
            for (Method method : c.getDeclaredMethods()) {
                if (!method.getName().startsWith("test") || !Pattern.matches(pattern, method.getName())) {
                    continue;
                }

                System.out.print(method.getName());
                try {
                    method.invoke(test);
                    System.out.println(" PASS");
                } catch (Exception ex) {
                    System.out.println(" FAIL");
                    ex.printStackTrace();
                    failed = true;
                }
            }
        }
    }

    private static class ExtraTest {
        public void testCopyFromScanDir() {
            Dummy d = new Dummy();
            assertEquals("Values must match", 42, d.fortyTwo);
        }
    }
}

